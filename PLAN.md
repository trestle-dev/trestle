# Trestle delivery plan

This programme builds Trestle as tested vertical slices. A phase is complete
only when its exit evidence exists; a UI mock or happy-path handler alone is
not completion. Dogfood the evolving platform with a small issue and incident
tracker so schema, auth, files, realtime, jobs, and backups meet a real use case.

## 0. Contracts and test harness

- Establish Go module, package boundaries, configuration precedence, structured
  logging, error vocabulary, clock/ID interfaces, and test fixtures.
- Record the threat model, API namespaces, database ownership rules, and browser
  viewport contract before implementation makes them expensive to change.
- Add CI for formatting, vet, tests, race tests, frontend syntax checks, Nift
  builds, and release builds.

Exit evidence: clean CI on an empty but runnable service; contract documents and
security test categories reviewed together; deterministic temporary-database
integration tests run without external services.

## 1. Single-node foundation

- Start an HTTP server, create/upgrade SQLite through numbered migrations, and
  expose narrowly scoped health/readiness endpoints.
- Bootstrap the first superuser without a permanent default credential.
- Build the Nift source and embed generated assets into the Go executable while
  retaining an explicit development override.
- Implement consistent shutdown, backup primitives, configuration validation,
  and redacted diagnostics.
- Produce Linux, macOS, and Windows archives for amd64 and arm64.

Exit evidence: a fresh executable initializes, restarts, migrates, serves its
embedded UI without a working directory, backs up consistently, and passes all
six cross-builds.

## 2. Collections and schema engine

- Store collection metadata while giving each collection a physical SQLite
  table. Begin with text, number, boolean, date/time, JSON, select, relation,
  email/URL, and generated metadata fields.
- Validate names and schema changes. Rebuild SQLite tables transactionally when
  alteration requires it, preserving indexes and recovering safely after a
  crash.
- Keep auth-collection semantics out of the first base-collection slice; add
  read-only view collections later.

Exit evidence: create/change/delete schema tests cover invalid transitions,
rollback, crash recovery, concurrent access, relation integrity, and migrations
from every released schema version.

## 3. Language-neutral record API

- Add versioned CRUD, pagination, sorting, projection, expansion, batch writes,
  and optimistic concurrency.
- Parse filters into a typed AST and compile only allow-listed constructs into
  parameterized SQL with complexity and result limits.
- Publish OpenAPI from the canonical server contract; SDKs must not acquire
  capabilities absent from plain HTTP.

Exit evidence: black-box HTTP tests exercise the API without a Go client;
injection/property tests cover the filter compiler; documented error and
pagination contracts are stable.

## 4. Authentication and authorization

- Add auth collections, password/session flows, revocation, and recovery.
- Introduce separate user sessions, service accounts, personal tokens,
  short-lived function grants, and superusers.
- Compile collection rules through the same safe expression pipeline and
  provide rule explanation for administrators without leaking sensitive data.

Exit evidence: an authorization matrix covers every actor and operation across
records, relations, batches, files, and realtime; revoked and expired identities
fail immediately where promised.

## 5. Files and object storage

- Define one storage interface with local filesystem first and S3-compatible
  storage second.
- Stage uploads, validate limits and names, bind metadata atomically, clean
  orphans, and authorize every read/delete independently of URL knowledge.
- Add optional signed delivery without treating signatures as collection rules.

Exit evidence: interrupted upload, rollback, traversal, content-disposition,
quota, orphan cleanup, and backend-parity tests pass.

## 6. Durable realtime

- Emit a monotonically sequenced event journal in the committing transaction.
- Stream through SSE, support `Last-Event-ID` replay within retention, bound
  slow consumers, and re-authorize data before delivery.
- Make retention and replay gaps explicit protocol states.

Exit evidence: reconnect/replay tests prove no silent gaps inside retention;
unauthorized fields and newly revoked access never leak; load tests bound memory
for slow or disconnected clients.

## 7. Administration dashboard

- Build the dark, fixed-viewport Nift/vanilla JS shell and collection, record,
  identity, file, realtime, audit, backup, and settings workflows.
- Keep scrolling inside named work panes and support keyboard navigation,
  focus restoration, reduced motion, and narrow-screen drawers.
- Show generated requests, rule explanations, and schema metadata to improve
  developer and AI-agent usability.

Exit evidence: automated viewport assertions show no page-level overflow at
supported sizes; critical workflows are keyboard-complete and pass accessibility
checks; the production executable serves only embedded release assets.

## 8. Durable jobs, webhooks, and AWS Lambda

- Add transactional outbox/jobs with leases, retries, backoff, dead-letter
  state, idempotency keys, retention, and operational visibility.
- Add signed webhooks and a provider-neutral function adapter; implement async
  AWS Lambda invocation first.
- Mint a narrow, short-lived function grant when callbacks are required.

Exit evidence: commit/rollback, worker crash, lease expiry, duplicate delivery,
retry, signature, credential-redaction, and callback-scope tests pass. Local
user code never executes in the Trestle server process.

## 9. Developer and AI experience

- Stabilize OpenAPI and generate a TypeScript client as a reference convenience,
  not a privileged interface.
- Add schema inspection, request examples, event schemas, migration planning,
  rule diagnostics, and machine-readable capability/version endpoints.
- Write task-oriented docs for curl and at least two unrelated languages.

Exit evidence: the dogfood app can be implemented from published HTTP docs in
multiple languages; generated artifacts reproduce in CI with no hand edits.

## 10. Import, operations, and compatibility boundary

- Build export/import with dry-run validation and resumable diagnostics.
- Consider a PocketBase importer as an explicit translation tool, never as API
  compatibility or a reason to inherit undocumented behavior.
- Add restore drills, maintenance operations, retention controls, and upgrade
  guidance.

Exit evidence: backup/restore and import tests compare records, files, identity
state, and schema; failures identify recoverable next actions.

## 11. Hardening and first stable release

- Complete fuzzing, race/load tests, dependency and artifact review, security
  headers, proxy deployment guidance, provenance, checksums, and upgrade tests.
- Freeze the supported contract and document all experimental surfaces.

Exit evidence: clean install and upgrade matrices pass on supported platforms;
threat-model tests and restore drills are green; release notes distinguish
implemented guarantees from roadmap work.

## Deliberately deferred

PostgreSQL, clustering, GraphQL, a plugin marketplace, a hosted cloud, exact
PocketBase compatibility, and a local user-code runtime are not first-release
requirements. Revisit them only with a concrete use case and an architecture
that preserves the single-node product's safety and clarity.
