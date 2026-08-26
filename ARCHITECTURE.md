# Trestle architecture

## Deployment shape

The first supported topology is one Go process owning one SQLite database and
serving an embedded Nift-built dashboard. External object storage and AWS Lambda
are optional adapters. This constraint is a feature: installation, backup,
upgrades, and failure ownership must remain understandable.

```text
clients ──HTTP/JSON/SSE──> Go server ──transactions──> SQLite
                              │
                              ├── files ──> local or S3-compatible storage
                              ├── jobs ───> webhooks or AWS Lambda
                              └── assets ─> embedded Nift output
```

Do not let optional providers become prerequisites for core records, auth,
files, realtime, or administration.

## Server boundaries

Prefer explicit internal packages with narrow dependencies:

- `cmd/trestle`: configuration, composition, signals, and process lifecycle.
- `internal/httpapi`: application, admin, and system transports.
- `internal/auth`: identities, credentials, sessions, grants, and revocation.
- `internal/policy`: parsed rule/filter ASTs, type checking, and evaluation.
- `internal/schema`: collection metadata, validation, and physical migrations.
- `internal/records`: transactional record operations and relation integrity.
- `internal/files`: metadata, staging, storage adapters, and cleanup.
- `internal/events`: committed event journal and SSE delivery.
- `internal/jobs`: outbox claiming, leases, retries, and retention.
- `internal/functions`: provider-neutral invocation and AWS adapter.
- `internal/audit`: security/administrative facts and redaction.
- `internal/store`: SQLite connection, migrations, transaction helpers, backup.
- `web`: generated assets and embedding boundary.

These are intended ownership seams, not a requirement to create empty packages.
Domain services must not depend on HTTP request objects or dashboard code.

## Persistence model

SQLite runs with foreign keys enabled, a finite busy timeout, and WAL where the
deployment filesystem supports its correctness requirements. Startup validates
the database and refuses unknown future schema versions. Migrations are ordered,
transactional where SQLite permits, and tested from every released version.

Each collection receives a physical table rather than storing all values in one
JSON blob. System metadata lives in reserved tables, including equivalents of:

- collections, fields, indexes, and migration history;
- identities, credentials, sessions, tokens, and grants;
- file metadata and staged uploads;
- event journal and subscriber/replay metadata;
- jobs, attempts, leases, schedules, and dead-letter state;
- audit entries and configuration metadata.

Exact names are an implementation decision, but reserve a prefix and prevent
user schemas from colliding with it. Schema alteration that needs table rebuild
must create/copy/validate/swap safely and leave enough state to diagnose or
recover after interruption.

## Transaction and event rule

A state change, its durable event, audit fact, and any required outbox job are
written in the same SQLite transaction. Workers act only after commit. Rollback
therefore produces neither an externally visible event nor a provider call.

Workers claim jobs with bounded leases. Every delivery has a stable event ID and
idempotency key; retries are expected. “Accepted by provider” and “completed by
user code” are distinct states and must not be conflated.

## API authority boundaries

- Application API: records, permitted files, auth-user flows, and realtime,
  evaluated under collection rules.
- Administration API: schemas, rules, identities, integrations, audit, backups,
  and operational controls, evaluated under explicit admin scopes.
- System endpoints: minimal health/readiness/version surfaces without application
  data. Diagnostics are authenticated when they reveal configuration.

Routes may share middleware, but authority never derives from URL obscurity.
Service accounts are the normal identity for trusted backends; superusers are
for human/bootstrap administration.

## Assets and overrides

Nift source is authored under the repository's content/templates/assets model.
Generated production files are embedded with Go's `embed` support and served
from the executable. A clearly named static-directory override may exist for
development and custom deployments; startup logs must make its use obvious.

## Scaling boundary

The initial consistency model assumes one process owns writes, jobs, and event
ordering. Do not imply clustering merely because SQLite can sit on shared
storage. A future multi-node design would need explicit database, lease, event,
cache, and object-storage contracts and is not a transparent configuration flip.
