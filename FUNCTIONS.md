# Event integrations and functions

Trestle begins by invoking external AWS Lambda functions from committed events.
It does not begin by hosting arbitrary user code. The core architecture is
provider-neutral so another remote executor can be added without changing event
or job semantics.

## Delivery path

1. A record/auth/file operation commits.
2. The same SQLite transaction writes its event and matching outbox job.
3. After commit, a worker claims the job with a lease.
4. The provider adapter invokes the configured target asynchronously.
5. Trestle records acceptance, attempts, latency, provider reference, and safe
   failure detail; retries obey bounded backoff and dead-letter policy.

A rolled-back application transaction cannot invoke a function. Delivery is at
least once, so handlers must use the stable event ID as an idempotency key.

## Provider boundary

Define an adapter around a small request/result contract rather than leaking AWS
types through domain packages. AWS Lambda is the first implementation. Initial
invocation is asynchronous; provider acceptance does not prove handler success.
If completion reporting is later required, make it a signed callback or event,
not a fabricated synchronous result.

Configuration includes target, region/endpoint policy, event filters, timeout at
the dispatch boundary, retry policy, concurrency, and credential reference.
Validate targets and prevent user-controlled arbitrary endpoints from becoming
SSRF or credential-forwarding mechanisms.

## Event envelope

Use a versioned CloudEvents-like JSON envelope while owning the Trestle schema:

```json
{
  "specversion": "1.0",
  "id": "evt_...",
  "type": "dev.trestle.record.created.v1",
  "source": "/projects/default/collections/issues",
  "time": "2026-08-26T08:00:00Z",
  "subject": "records/rec_...",
  "datacontenttype": "application/json",
  "data": {
    "project": "default",
    "collection": "issues",
    "recordId": "rec_...",
    "actor": {"kind": "user", "id": "usr_..."},
    "changes": ["status"]
  },
  "trestle": {"delivery": "job_...", "attempt": 1}
}
```

Minimize payload data by default. Secrets, credential material, hidden fields,
and values the integration is not allowed to access never enter the envelope.
Large data should be fetched through scoped callbacks rather than copied blindly.

## Callback authority

When a function must call Trestle, mint a short-lived function grant scoped to
the target, invocation, audience, project, and exact permitted operations. Prefer
an exchange/callback mechanism that avoids durable credentials in event logs.
Revocation and expiry are enforced independently of provider success.

AWS credentials come from the normal server credential chain or an explicit
secret reference. Never store plaintext credentials in collection data, event
payloads, job diagnostics, audit details, or browser responses.

## Operational UI

Administrators need event filters, enabled state, recent deliveries, attempts,
next retry, dead-letter reason, correlation/request IDs, payload schema, replay
controls, and redacted provider diagnostics. Manual replay creates a new delivery
attempt with attribution; it does not rewrite history.

## Deferred local runtime

Do not evaluate JavaScript, WASM, shell, templates, or plugins inside the main Go
process. A future local runtime needs a separate worker process, resource limits,
filesystem/network capabilities, secret scoping, protocol versioning, cancellation,
audit, and credible platform isolation. Until that design exists, remote function
invocation is the honest boundary.
