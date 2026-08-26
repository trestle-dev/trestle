# Language-neutral API contract

Trestle's canonical interface is versioned HTTP with JSON and Server-Sent Events.
Anything supported by an official SDK must be possible without that SDK. Publish
OpenAPI and event schemas from the same contracts exercised by server tests.

## Namespaces

- `/api/v1/...`: application records, users, permitted files, and realtime.
- `/admin/v1/...`: schemas, policies, service identities, jobs, audit, and
  operations.
- `/system/...`: small health, readiness, and version/capability endpoints.

Version semantics must be documented before routes are declared stable. Admin
credentials never silently elevate requests sent to application routes.

## Resource direction

An initial shape may include:

```text
GET    /api/v1/collections/{collection}/records
POST   /api/v1/collections/{collection}/records
GET    /api/v1/collections/{collection}/records/{id}
PATCH  /api/v1/collections/{collection}/records/{id}
DELETE /api/v1/collections/{collection}/records/{id}
GET    /api/v1/events
```

Collection names and record IDs are path segments, not SQL identifiers. Resolve
them through validated metadata. Mutations return version metadata suitable for
conditional updates; support an idempotency key on operations where retry could
otherwise duplicate effects.

## Responses and errors

Use one predictable error envelope, for example:

```json
{
  "error": {
    "code": "validation_failed",
    "message": "The request could not be applied.",
    "requestId": "req_...",
    "details": [{"path": "title", "code": "required"}]
  }
}
```

Codes are stable program inputs; prose is not. Never expose SQL, filesystem
paths, provider secrets, stack traces, password/token existence, or policy data
the actor cannot inspect. Define status codes for validation, authentication,
authorization, conflict, precondition failure, rate limit, and replay gaps.

## Querying

Support bounded page size, deterministic sorting with an ID tie-breaker, field
projection, and explicitly limited relation expansion. Cursor pagination is the
long-term default; if offset pagination ships first, state its consistency limits.

Filters use a documented expression grammar parsed into a typed AST. Validate
fields/operators/types/complexity, then compile allow-listed nodes to bound SQL
and parameters. The same principle applies to access rules. Raw SQL fragments
are never accepted through application APIs.

## Identities

- User sessions represent application users and are evaluated by collection
  policy.
- Service accounts represent trusted application backends and receive explicit,
  revocable scopes.
- Personal tokens represent an administrator for tooling and remain attributable.
- Function grants are short-lived, audience-bound, and limited to one invocation
  or declared callback capabilities.
- Superusers perform bootstrap and exceptional administration, not app traffic.

Document header/cookie use, expiry, rotation, revocation, CORS, and CSRF behavior
per identity. Never place long-lived credentials in query strings.

## Realtime SSE

The event stream uses stable event types, IDs, schema versions, and `id:` fields.
Clients reconnect with `Last-Event-ID`. Trestle replays authorized retained events
in order or returns an explicit replay-gap response that tells the client to
resynchronize. Subscription filters never broaden the actor's record permissions.

Heartbeats, connection limits, maximum buffered bytes, retention, and slow-client
closure are protocol behavior and require documentation and tests.

## Clients and compatibility

Generate client conveniences only after the HTTP contract works through curl.
Do not add SDK-only admin shortcuts. Backward compatibility refers to Trestle's
published versions, not PocketBase behavior; any future importer is a separate,
explicit translation workflow.
