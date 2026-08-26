# CP21 dogfood findings

CP21 used a temporary incident-management client to exercise Trestle strictly
through its published HTTP interfaces. The client and its seed harness were
removed after the checkpoint because they were validation machinery, not a
polished example that belonged in the core repository. The checkpoint commit
retains the exact implementation and this document retains the resulting
product findings.

## Exercised product paths

- first-run administrator setup and CSRF-protected administration;
- two typed collections and text-backed relations;
- application-user registration, login, access tokens, refresh logout and
  owner-aware collection rules;
- record creation, querying, idempotency and related timeline records;
- scoped service credentials and authenticated file upload/download;
- durable event creation and SSE replay using `Last-Event-ID`;
- audit inspection and both successful and retrying durable jobs;
- webhook configuration/disable and AWS Lambda missing-credential diagnostics;
- consistent backup creation, download and restore preflight.

## Product decisions from the exercise

1. **Remove the false project selector.** Trestle is currently one deployment
   and one SQLite database. `Project / Default` implied unsupported tenancy.
2. **Replace stale overview scaffolding.** The dashboard still described the
   CP03 shell and deferred screens that have long since shipped.
3. **Separate webhook configuration from delivery DNS.** An HTTPS target can be
   saved while offline. Every actual delivery still resolves DNS, refuses
   private/link-local destinations, refuses redirects and bounds the response.
4. **Keep relation behavior explicit.** CP21 uses relation values as record IDs
   and performs a second authorized query. Target-aware foreign keys and inline
   expansion remain documented limitations, not hidden fixtures.
5. **Keep provider credentials out of the example.** The Lambda target exercises
   durable retry diagnostics without shipping AWS secrets. A real acceptance
   result requires operator-supplied credentials and a real AWS account.
6. **Keep dogfood clients outside the core repository.** Future end-to-end
   applications should live in their own repositories, where their product and
   release quality can be evaluated independently from Trestle itself.
