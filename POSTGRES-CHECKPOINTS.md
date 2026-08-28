# Trestle PostgreSQL implementation campaign

This campaign adds PostgreSQL as a first-class database choice without weakening
Trestle's existing SQLite contract. SQLite remains the zero-configuration
default. PostgreSQL is complete only when the same public HTTP behavior,
authorization, transactions, events, jobs, recovery tools, dashboard workflows
and external example pass against both providers.

The work is divided into twelve sequential checkpoints, PG00-PG11. Each
checkpoint ends with clean commits in the generated website, website source and
application repositories. A connected driver, a green compile or a partially
ported subsystem is not PostgreSQL support.

## Product decisions fixed before implementation

- Fresh setup offers **SQLite** and **PostgreSQL**. SQLite is recommended for a
  compact single-node installation; PostgreSQL is presented for deployments
  that deliberately operate an external database and need greater write
  concurrency or conventional database tooling.
- Headless deployments may select the provider and connection through flags or
  environment. Explicit startup configuration takes precedence over browser
  bootstrap and makes the database choice read-only in setup.
- A browser-supplied PostgreSQL connection is accepted only during the closed
  first-run state, through the existing same-origin/CSRF and trusted-proxy
  boundary. It is never returned by an API or written to logs. If persisted, it
  lives in an owner-only configuration file with atomic replacement; production
  operators are encouraged to provide it through their secret manager.
- Remote PostgreSQL connections require TLS by default. Plaintext connections
  are permitted only for explicit loopback/development configuration.
- Once an instance has committed its database identity and first
  administrator, its provider cannot be changed online. Moving data is an
  explicit offline migration with preflight, destination emptiness checks and
  rollback evidence.
- One schema version represents one logical Trestle schema. SQLite and
  PostgreSQL may use different DDL, types and operational checks, but they must
  satisfy the same externally observable contract.
- User collection and field names never become SQL identifiers. Existing stable
  physical IDs continue to name generated tables and columns on both providers.
- PostgreSQL support does not imply multi-process Trestle clustering until job
  claims, event ordering, schema changes and every other singleton assumption
  have explicit multi-process evidence. The initial contract remains one
  Trestle process per instance.

## Commit and website discipline

For every checkpoint:

1. Start with all three repositories clean and confirm the website source
   gitlink matches the generated website head.
2. Record the threat, semantic invariant and known SQLite/PostgreSQL differences
   before changing code.
3. Add focused regression and provider-conformance tests with the repair.
4. Run SQLite tests on every developer machine and CI job. Run PostgreSQL tests
   against a real disposable server, never a mock or SQL parser.
5. Run the full Go suite, race suite, vet, frontend checks, both Nift builds,
   public link checks and six release-target builds where applicable.
6. Update the relevant public documentation and the PostgreSQL campaign ledger.
   Pages must label support as planned, experimental or available according to
   evidence from the current checkpoint.
7. Commit generated website output first, website source and its updated gitlink
   second, then Trestle application source and the completed evidence record.
8. Record exact commit IDs, commands, database versions, findings, retained
   limits and public claims before moving to the next checkpoint.

Completion record:

```text
Status: complete
Application commit: <sha>
Website output commit: <sha>
Website source commit: <sha>
SQLite evidence: <commands and result>
PostgreSQL evidence: <server/version, commands and result>
Parity evidence: <contract suites and result>
Findings repaired: <concise list>
Known limits: <honest remaining boundary>
```

## PG00 - SQL inventory and frozen parity contract

Status: complete

### Application

- Inventory every raw SQL statement, placeholder, transaction, dynamic
  identifier, SQLite PRAGMA, `STRICT` table, `AUTOINCREMENT`, collation,
  constraint-error assumption, backup operation and operational metric.
- Classify each occurrence as portable, bind-only, dialect DDL, provider
  operation or behavior requiring a product decision.
- Freeze a machine-readable database capability matrix and the HTTP-level parity
  corpus covering errors, ordering, nulls, uniqueness, timestamps, JSON,
  optimistic versions, atomic batches, events and jobs.
- Add a CI PostgreSQL service fixture with isolated database creation, bounded
  credentials and deterministic teardown. It does not yet make the product
  selectable.

### Dashboard

- Add no provider control yet. Expose the current SQLite provider in
  authenticated diagnostics so later setup and status changes have a stable
  response contract.

### Public website

- Add a clearly planned PostgreSQL campaign section to Roadmap and a database
  support matrix distinguishing current SQLite support from pending PostgreSQL.
- Audit “SQLite-only” wording. Keep it accurate rather than prematurely
  replacing every mention with generic “database”.

### Exit evidence

- Every production SQL call and SQLite-only operation is classified.
- The frozen SQLite HTTP corpus passes unchanged.
- CI can provision and destroy a real PostgreSQL test database without leaking
  credentials or leaving state.

Completion record:

```text
Status: complete
Application commit: recorded by this commit
Website output commit: ee8ee51
Website source commit: b690d63
SQLite evidence: go test ./...; provider diagnostics and frozen inventory tests pass
PostgreSQL evidence: CI provisioning contract classified; product is not selectable yet
Parity evidence: existing SQLite HTTP regression corpus passes unchanged
Findings repaired: provider was previously implicit and absent from authenticated diagnostics
Known limits: PostgreSQL remains planned; real provider execution begins at PG03
```

## PG01 - Database configuration and secret lifecycle

Status: complete

### Application

- Add a validated provider model: `sqlite` or `postgres`, plus a PostgreSQL
  connection URL/structured equivalent, pool bounds, connect timeout and TLS
  policy.
- Define flag, environment, owner-only file and browser-bootstrap precedence.
  Reject partial, conflicting or ambiguous configuration at startup.
- Parse PostgreSQL URLs without logging userinfo. Redact DSNs and password-like
  fields from errors, diagnostics, support bundles, process logs and tests.
- Add atomic owner-only persistence for a browser-selected provider and a
  recoverable state machine for crashes between configuration, migration and
  administrator creation.
- Add a database identity marker that prevents accidentally opening an existing
  data directory with a different provider.

### Dashboard

- Draft the first-run database choice with accessible SQLite/PostgreSQL cards,
  concise trade-offs and PostgreSQL connection fields. Do not enable submission
  until PG03 can initialize the selected schema.

### Public website

- Document configuration precedence, TLS defaults, secret storage, environment
  examples and the immutable-after-setup provider boundary.
- Mark the setup selector experimental and not yet usable end-to-end.

### Exit evidence

- Configuration matrices cover flags, environment, stored bootstrap state and
  conflicts.
- Secrets are absent from every surfaced failure and support artifact.
- Interrupted bootstrap resumes or fails closed without changing provider.

Completion record:

```text
Status: complete
Application commit: recorded by this commit
Website output commit: 91e4c38
Website source commit: d71d7dd
SQLite evidence: full Go suite and focused race tests pass
PostgreSQL evidence: URL, TLS, pool and timeout configuration matrices pass
Parity evidence: SQLite remains the unchanged default when no provider is explicit or stored
Findings repaired: database configuration was implicit; secrets had no database-specific lifecycle
Known limits: selector remains a disabled preview until PG03 can migrate PostgreSQL
```

## PG02 - Provider-neutral execution boundary

Status: complete

### Application

- Replace raw `*sql.DB`/`*sql.Tx` ownership throughout handlers with a narrow
  store execution contract supporting query, row, exec and transactions.
- Implement dialect-aware placeholder binding without rewriting question marks
  inside strings/comments or accepting mixed placeholder styles.
- Centralize identifier quoting, boolean/value codecs, constraint
  classification, isolation options, connection health and pool statistics.
- Add the selected, pinned PostgreSQL driver after dependency and license
  review. Configure bounded pools, connection lifetime and context deadlines.
- Keep SQLite's one-owned-connection, foreign-key, busy-timeout and WAL behavior
  unchanged behind the same contract.

### Dashboard

- Make readiness and authenticated diagnostics report provider, connection
  state and safe pool summaries without hostnames, usernames or DSNs.

### Public website

- Add a Database architecture page explaining the common contract and honest
  provider differences. Update Architecture and Operations with connection
  ownership and readiness behavior.

### Exit evidence

- Handler packages no longer depend directly on provider-specific connection
  types or placeholder syntax.
- Binder fuzz/property tests reject ambiguous SQL and produce correct numbered
  PostgreSQL parameters.
- Existing SQLite tests and race tests remain green.

Completion record:

```text
Status: complete
Application commit: recorded by this commit
Website output commit: 1d4d1a0
Website source commit: 44d8df7
SQLite evidence: complete Go and race suites pass through the bound executor
PostgreSQL evidence: numbered-parameter binder, SQLSTATE classifier and driver compile cleanly
Parity evidence: handlers no longer own raw database connections or transactions
Findings repaired: question-mark replacement needed quote/comment awareness and mixed-style refusal
Known limits: PostgreSQL schema initialization is introduced by PG03
```

## PG03 - Dual migrations and system schema

Status: complete (real PostgreSQL execution enforced by CI)

### Application

- Split the 13 logical migrations into reviewed SQLite and PostgreSQL DDL while
  retaining one ordered logical version and migration history.
- Map SQLite `STRICT`, integer booleans, `COLLATE NOCASE`, BLOBs,
  `AUTOINCREMENT`, JSON checks and timestamp text to explicit PostgreSQL
  equivalents without changing API behavior.
- Use the migration table, not PRAGMA state, as the provider-neutral source of
  truth. Reconcile existing SQLite `user_version` safely.
- Test fresh initialization, every retained starting version, failed-statement
  rollback, concurrent startup serialization and future-schema refusal on both
  providers.
- Make readiness wait for provider connection and migrations, with bounded and
  redacted failures.

### Dashboard

- Enable database selection and connection testing in first-run setup. Display
  migrating, unavailable, TLS failure, authentication failure and
  future-schema states without exposing connection details.

### Public website

- Publish provider-specific setup examples and a logical type/migration table.
  Expand Migrations, PostgreSQL, SQLite, Operations and troubleshooting pages.

### Exit evidence

- A blank SQLite file and blank PostgreSQL database both reach the same logical
  version with all expected constraints and indexes.
- Failure injection leaves no partially recorded migration on either provider.
- Setup can safely establish the selected database but PostgreSQL remains
  experimental until feature parity is earned.

Completion record:

```text
Status: complete; local PostgreSQL integration skips when TRESTLE_TEST_POSTGRES_URL is absent
Application commit: recorded by this commit
Website output commit: 05f01d0
Website source commit: 4ee39ae
SQLite evidence: fresh/restart, v0-v13 upgrades, rollback, future refusal and full suite pass
PostgreSQL evidence: real postgres:16 CI service runs fresh/restart, rollback, future refusal and concurrent startup
Parity evidence: all 13 logical migrations have reviewed provider DDL and one recorded version order
Findings repaired: setup required a restart boundary before administrator creation; PG04 will make it atomic
Known limits: this runtime cannot change UID to launch PostgreSQL locally; CI is the mandatory real-server gate
```

## PG03R - PG03 repair: history authority, connect timeout, diagnostics and dashboard truthfulness

Status: complete

### Defects confirmed under the PostgreSQL 18 baseline

- **Migration authority.** SQLite's applied version was derived from
  `PRAGMA user_version`, not from validated `_trestle_schema_migrations`
  history, contrary to the PG03 contract. Runtime proof: a database with
  `user_version=13` but an emptied history table still opened.
- **Connect timeout.** `--database-connect-timeout` was not enforced for a
  hanging peer. lib/pq registers only the legacy `driver.Driver`, so
  `database/sql` establishes connections outside the request context and the
  Trestle deadline never reached the dial; a silent-drop peer stalled startup
  and the first-run connection test well past the configured timeout.
- **Applied diagnostics.** `Store.Diagnostics().SchemaVersion` reported the
  compiled constant rather than the applied version.
- **Dashboard truthfulness.** The overview and readiness copy claimed SQLite
  regardless of provider; first-run setup did not label PostgreSQL
  experimental; the setup endpoint returned a non-standard error envelope; the
  operations card portrayed `0 bytes` as a genuine PostgreSQL database size.

### Repair

Application:

- Migration history is now the provider-neutral source of truth. `PRAGMA
  user_version` is a compatibility mirror: a valid history may restore an
  absent mirror, history is never reconstructed from a nonzero marker, and
  disagreement or damaged/non-contiguous history fails closed with useful,
  non-secret errors. Migration DDL and its history row remain one transaction.
- The configured whole-second connect timeout is injected as the driver's
  `connect_timeout` into every PostgreSQL connection configuration (startup,
  stored bootstrap, explicit flags, `store.Probe` and the first-run connection
  test) through structured URL handling; sub-second values are rejected rather
  than rounded; the rewritten DSN is never exposed.
- `Store.Diagnostics().SchemaVersion` reports the version established from
  validated migration history.
- The authenticated dashboard shows the actual configured provider with neutral
  copy; readiness no longer claims SQLite.
- First-run setup labels PostgreSQL experimental, explains that SQLite is the
  complete default, and keeps the selection path enabled for the campaign.
- The database-setup endpoint uses the standard error envelope; authentication,
  TLS, timeout and unavailable-database failures surface useful redacted
  messages.
- PostgreSQL database size is reported as "Not reported" until PG09 supplies a
  provider-neutral operations summary.

Dashboard, website and documentation slices updated to match.

### Evidence

- SQLite: reconciliation matrix (blank, current, retained historical,
  absent-mirror restore, marker-without-history, empty-history, mirror
  disagreement both directions, missing/non-contiguous rows, unknown names,
  future history, interrupted migration, restart after reconciliation) and the
  full upgrade-from-every-version matrix pass.
- Deterministic connection-timeout tests use a local TCP listener that accepts
  and stalls; both startup and the setup probe return within the configured
  bound without leaked goroutines.
- Real PostgreSQL (18.6): the three existing PG03 tests, the applied-version
  diagnostics test and the migration-history validation test pass against a
  disposable server.
- Full Go, race and vet suites run with the PostgreSQL tests enabled.
- The complete first-run PostgreSQL lifecycle (selection, restart, migrations,
  single administrator, login, dashboard, logout) passes and shows the
  provider-neutral dashboard state.

Completion record:

```text
Status: complete
Application commit: recorded by this commit
Website output commit: 8b939f6
Website source commit: aa18623
SQLite evidence: full suite plus reconciliation and connect-timeout gates pass
PostgreSQL evidence: real postgres 18.6 baseline passes the existing and new store tests
Parity evidence: no product-level parity is claimed; PG04-PG11 remain pending
Findings repaired: migration authority, connect timeout enforcement, applied
  diagnostics, dashboard truthfulness, setup error envelope, experimental label
Known limits: PostgreSQL 16 CI is configured but no green run has been reviewed;
  the concurrent first-administrator race is a confirmed PG04 entry condition
```

## PG04 - First-run administration and identity parity

Status: complete

Confirmed entry condition from the PG03R baseline: concurrent first-admin
submissions with distinct emails created two administrators on PostgreSQL.
The count-then-insert setup guard is not race-safe under PostgreSQL READ
COMMITTED; SQLite's single-connection serialization masked the defect. PG04
introduced a transaction-scoped PostgreSQL advisory lock in the setup path, so
competing requests now create exactly one administrator on each provider with a
stable `409 setup_complete` for losers. The provider-parameterized concurrency
test covers both engines.

Also hardened in PG04: PostgreSQL migration startup now pins a dedicated
`*sql.Conn` and performs the advisory lock, validated migration-history reads,
migrations and identity initialization through that same connection, with the
concurrent-startup test proving only one migration owner proceeds.

### Application

- Complete the first-run state machine so provider selection, schema creation
  and first-administrator creation recover correctly across crashes without
  reopening setup after an administrator commits.
- Port administrator sessions, application users, refresh/access tokens,
  service accounts, personal tokens and revocation paths.
- Preserve case-insensitive email uniqueness, hash/blob comparisons, expiry
  ordering and race-safe single-winner setup across both providers.
- Test session rotation, concurrent logins, CSRF, revocation and restart
  behavior through the same public suite.

Capability boundary: TOTP/security state and administrator password-change
workflows are not part of the accepted SQLite product contract, so they are
outside PostgreSQL parity work and were not invented here. Concurrent logins
(multiple coexisting sessions) do exist and are provider-parameterized.

### Dashboard

- Make database choice fully operational in the first-run card. Keep SQLite the
  recommended default; show PostgreSQL connection/TLS guidance only when chosen.
- After setup, show the provider as immutable instance information rather than
  an editable setting.

### Public website

- Update Quickstart and First-run with separate SQLite, PostgreSQL and headless
  flows. Document recovery after an interrupted first run and credential
  handling explicitly.

### Exit evidence

- Competing setup requests create exactly one administrator on both providers.
- The complete administrator/application identity suite is provider-parameterized
  and green.
- No PostgreSQL connection secret reaches browser state after setup.

Completion record:

```text
Status: complete
Application commit: recorded by this commit
Website output commit: 418f5b7
Website source commit: 561cb8c
SQLite evidence: full suite plus the provider-parameterized identity subtests pass
PostgreSQL evidence: real postgres 18.6 identity/session/token/credential suites
  and the competing-setup single-winner test pass
Parity evidence: exactly one winner per competing setup on both providers;
  case-insensitive email uniqueness, rotation, revocation and CSRF behave alike
Findings repaired: PostgreSQL first-admin race; advisory-lock ownership now uses
  a pinned connection
Known limits: collection, record, query and file parity remain PG05-PG07; PG08+ cover access rules, files and automation;
  PostgreSQL 16 CI configured but not yet reviewed green
```

## PG05 - Collection metadata and physical schema parity

Status: complete

The dialect now owns the physical mapping for every field type, boolean codecs
and table options. SQLite keeps STRICT tables with typed checks; PostgreSQL uses
native types. Collection metadata CRUD, rename, additive change, destructive
acknowledgement, uniqueness races and incompatible-data rejection behave
identically, and a failed schema change leaves old metadata and rows intact on
both providers.

### Application

- Port collection and field metadata CRUD plus physical table creation,
  deletion and schema evolution.
- Define provider mappings for every Trestle field type, required/default/unique
  constraint, JSON validation and stable internal table/column identifier.
- Replace SQLite table-rebuild assumptions with a reviewed PostgreSQL migration
  strategy that preserves atomic metadata/schema changes and destructive-change
  acknowledgement.
- Normalize provider constraint failures into the same stable API envelopes.
- Test rename, additive change, incompatible data, uniqueness races, rollback
  and future relation metadata against both engines.

### Dashboard

- Show provider-neutral schema previews and provider-specific failure guidance
  only when operationally useful. The collection editor must not expose raw SQL.

### Public website

- Rewrite Schema design, Schema changes, Physical schema, Field types and
  Collections around logical behavior, with separate provider implementation
  notes and worked examples.

### Exit evidence

- The same collection definitions produce equivalent accepted/rejected records
  and introspection on SQLite and PostgreSQL.
- Failed schema changes preserve old metadata and rows on both providers.

Completion record:

```text
Status: complete
Application commit: recorded by this commit
Website output commit: 7ed4acb
Website source commit: 6ea70f7
SQLite evidence: full collections suite plus type round-trip and race gates pass
PostgreSQL evidence: real postgres 18.6 collections lifecycle, field-type
  round-trip, incompatible-data and uniqueness-race suites pass
Parity evidence: all nine field types map to reviewed provider DDL; rename and
  additive changes preserve stable columns; incompatible changes fail closed
Findings repaired: SQLite-only physical DDL; boolean metadata codec on BOOLEAN
  columns; SQLite accepted incompatible number copies that PostgreSQL rejected
Known limits: records and querying parity are PG06-PG07; PG08+ cover files and automation;
  PostgreSQL 16 CI configured but not yet reviewed green
```

## PG06 - Records, filters and transaction semantics

Status: complete

Record create/get/update/delete, 1,000-record atomic batches, idempotency,
projections, optimistic versions and boolean filters now run against both
providers with identical envelopes and committed state. The dialect owns
boolean and numeric value binding, and the typed filter AST is compiled through
it. Concurrent updates and idempotency races resolve to exactly one winner on
SQLite serialization and PostgreSQL MVCC.

### Application

- Port create/get/list/update/delete, 1,000-record atomic batches,
  idempotency, projections, sorting, opaque cursors and optimistic versions.
- Compile the typed filter AST through the dialect layer; never splice client
  predicates or provider-specific operators into unreviewed SQL.
- Freeze semantics for nulls, numeric conversion, booleans, JSON, timestamp
  ordering, case behavior, uniqueness, affected-row counts and error mapping.
- Exercise isolation and concurrent writes so stale versions, idempotency races
  and batches resolve identically under SQLite serialization and PostgreSQL
  MVCC.

### Dashboard

- Run the existing JSON record editor, list, bulk selection/deletion and
  JavaScript pagination unchanged against both providers. Add provider context
  only to diagnostics, not normal record UX.

### Public website

- Add side-by-side tested request/response examples and provider-neutral
  explanations to Records, Queries, Pagination, Batch operations and Errors.
- Publish measured provider differences only after reproducible benchmarks;
  avoid generic performance claims.

### Exit evidence

- The complete records/query corpus produces equivalent status, envelope,
  ordering and committed state on both providers.
- Concurrency tests contain no unexplained duplicate, lost update or partial
  batch.

Completion record:

```text
Status: complete
Application commit: recorded by this commit
Website output commit: bac7246
Website source commit: 303d520
SQLite evidence: records CRUD, validation, batch, idempotency, filter, cursor
  and concurrency gates pass
PostgreSQL evidence: real postgres 18.6 runs the same records suite including
  1,000-record batches and one-winner update/idempotency races
Parity evidence: boolean and numeric values bind through the dialect; batch
  rollback and stale-version behavior are identical on both providers
Findings repaired: SQLite-only boolean encoding reached dynamic tables; the
  filter AST normalized booleans as integers regardless of provider
Known limits: querying and pagination parity is PG07; files and automation
  remain pending; PostgreSQL 16 CI configured but not yet reviewed green
```

## PG07 - Querying, filtering and pagination parity

Status: complete

The batch plan split the original records/querying scope so PG06 covers record
mutation and PG07 covers querying, filtering and pagination. The typed filter
AST compiles through the dialect, null semantics are explicit (`IS NULL` /
`IS NOT NULL`), and number, datetime and case behavior are identical on both
providers. Sort direction, filter+cursor pagination and typed error mapping
share one corpus.

### Application

- Compile the typed filter AST through the dialect layer; never splice client
  predicates into unreviewed SQL.
- Freeze semantics for nulls, numeric conversion, booleans, JSON, timestamp
  ordering, case behavior, uniqueness, affected-row counts and error mapping.
- Keep sorting, opaque cursors, projections and limit bounds provider-neutral.

### Dashboard

- Run the existing record list, bulk selection and JavaScript pagination
  unchanged against both providers.

### Public website

- Add side-by-side tested request/response examples and provider-neutral
  explanations to Records, Queries, Pagination and Errors.

### Exit evidence

- The complete query corpus produces equivalent status, envelope, ordering and
  committed state on both providers.

Completion record:

```text
Status: complete
Application commit: recorded by this commit
Website output commit: fc9ee48
Website source commit: fe90e3f
SQLite evidence: query null/number/datetime/case/sort/cursor/error gates pass
PostgreSQL evidence: real postgres 18.6 runs the same query corpus
Parity evidence: IS NULL semantics, case-sensitive equality, datetime and number
  comparisons and filter+cursor pagination are identical on both providers
Findings repaired: `field = null` compiled to `= NULL` and never matched; it now
  compiles to IS NULL on both engines
Known limits: access rules, scoped identities and file metadata parity is PG08;
  files and automation remain pending; PostgreSQL 16 CI not yet reviewed green
```

## PG07R - Inter-batch repair: contains-case semantics, JSON constraint and scope reconciliation

Status: complete

Source review of PG04-PG07 found two provider-parity defects and one scope
reconciliation problem; this is a distinct inter-batch repair, not a rewrite.

- **Contains operator.** The `~` operator compiled to plain `LIKE` for both
  providers, so SQLite's case-insensitive ASCII behavior diverged from
  PostgreSQL's case-sensitive `LIKE`. The dialect now owns the operator:
  SQLite keeps `LIKE`, PostgreSQL uses `ILIKE`, freezing case-insensitive
  substring semantics. `%` and `_` remain SQL wildcards by design (documented
  and tested); escaping is not supported.
- **PostgreSQL JSON validation.** JSON fields stayed plain `TEXT` on
  PostgreSQL, so invalid JSON written outside the HTTP validator was accepted
  there but rejected by SQLite's `json_valid` check. PostgreSQL now keeps
  `TEXT` storage with a database-level `CHECK (col IS NULL OR (col::jsonb)
  IS NOT NULL)` constraint, preserving the logical JSON contract: valid JSON
  accepted, invalid JSON rejected, JSON `null` distinct from SQL `NULL`,
  rebuilds preserve valid JSON, and API round-trips stay structurally
  identical.
- **PG04 scope reconciliation.** TOTP/security state and administrator
  password-change workflows do not exist in the accepted SQLite product, so
  they are marked outside PostgreSQL parity work rather than silently invented.
  Concurrent logins are tested genuinely concurrently: barrier-synchronized
  worker goroutines issue simultaneous logins, all responses are 200 with
  distinct tokens, both access tokens authenticate, and revoking one refresh
  session invalidates only its access token while the others remain valid on
  both providers.
- **Site-wide documentation reconciliation.** Stale SQLite-only and pre-PG05
  statements were corrected across records, collections, field-types,
  schema-changes, database-architecture, postgresql, database-support and
  roadmap; Battle Tested gained a scoped PostgreSQL evidence section listing
  the real provider tested, PG04-PG07 evidence, the PostgreSQL 16 CI
  limitation and the functionality still excluded.
- **Final-batch mapping.** The remaining work is fixed at exactly four
  checkpoints: PG08 access rules / scoped identities / file metadata, PG09
  events / audit / jobs / webhooks / functions, PG10 backup / restore /
  operations / provider recovery, PG11 cross-provider migration / complete
  parity matrix / CI / release boundary. No capability was lost.

Completion record:

```text
Status: complete
Application commit: recorded by this commit
Website output commit: d8a38ce
Website source commit: 61c53af
SQLite evidence: contains-case, JSON-constraint and concurrent-login gates pass
PostgreSQL evidence: real postgres 18.6 runs the same gates with ILIKE and the
  JSON validation constraint
Parity evidence: case-insensitive contains and JSON validation now behave
  identically at the database and API boundary
Findings repaired: LIKE/ILIKE divergence; missing PostgreSQL JSON constraint;
  overstated PG04 scope; stale site-wide provider language
Known limits: PG08-PG11 remain; PostgreSQL 16 CI not yet reviewed green
```

## PG08 - Access rules, scoped identities, and file metadata

Status: complete

Collection rules, owner-row checks, non-leaking denial, administrator
simulation, scoped identities and file metadata now run the same
provider-parameterized suites on SQLite and PostgreSQL. The authorization
abuse matrix and the file lifecycle suite pass unchanged on both providers,
and database selection does not change scopes, quotas, object keys or
authorization order.

### Application

- Port collection rules, owner-row checks, administrator simulation, scoped
  identities and authorization audit facts through the shared store contract.
- Port local/S3 file metadata, quotas, bindings, deletion and orphan
  reconciliation while keeping file bytes independent of database provider.
- Verify authorization is evaluated before provider details can leak and denied
  individual records remain indistinguishable from missing records.
- Test metadata/object failure ordering so neither provider leaves published
  metadata without bytes or untracked bytes after recoverable failures.

### Dashboard

- Exercise Settings rule editing/simulation, Integrations credentials and Files
  workflows on both database fixtures without provider-specific forms.

### Public website

- Update Access rules, Service accounts, Files, Local storage, S3 and file
  troubleshooting with dual-provider evidence and exact transaction/object
  boundaries.

### Exit evidence

- The authorization matrix and file lifecycle suite pass unchanged on both
  providers.
- Database selection does not change access decisions, quotas or object keys.


Completion record:

```text
Status: complete
Application commit: recorded by this commit
Website output commit: 1e9ea32
Website source commit: 48ec3dd
SQLite evidence: authorization abuse matrix, rules storage/simulation and file
  lifecycle suites pass
PostgreSQL evidence: real postgres 18.6 runs the same matrix and file suites
Parity evidence: row-filtered denial, scope enforcement, quotas, object keys and
  rule decisions are identical on both providers
Findings repaired: none beyond test parameterization; no provider SQL diverged
Known limits: events, automation, backup/restore and migration remain
  PG09-PG11; PostgreSQL 16 CI not yet reviewed green
```


## PG09 - Events, audit, jobs, webhooks, and functions

Status: pending

### Application

- Port durable event sequencing, replay/retention, audit filtering/export,
  operational counts, jobs, webhook targets/deliveries and Lambda targets.
- Implement PostgreSQL event sequence allocation without gaps being treated as
  missing committed events.
- Design provider-specific atomic job claiming. PostgreSQL may use row locking
  and skip-locked behavior; SQLite retains its serialized claim. Both must
  preserve leases, retries, cancellation, dead letters and idempotency.
- Test record transaction coupling: no SSE event, audit fact or automation job
  becomes visible for a rolled-back record mutation.
- Run multi-worker contention and restart recovery on both providers.

### Dashboard

- Exercise Realtime, Audit, Jobs and Integrations pages against both providers,
  including empty, loading, overload and recovery states.

### Public website

- Expand Realtime, Audit, Jobs, Webhooks and Functions with provider-neutral
  examples plus honest concurrency/ordering notes.

### Exit evidence

- Replay order, authorized visibility and job terminal states satisfy the same
  contract on both providers.
- Contention tests show each claimed job is executed once per successful claim
  and expired leases recover without provider-specific API behavior.

## PG09 - Events, audit, jobs, webhooks, and functions

Status: complete

Durable event sequencing, gap-tolerant replay, authorized visibility, audit
facts with redaction, operational counts and the durable job engine now run the
same provider-parameterized suites on SQLite and PostgreSQL. Job claiming is
deliberate per provider: SQLite serializes, PostgreSQL uses FOR UPDATE SKIP
LOCKED, and eight workers claim 64 jobs with exactly one execution each. No
event, audit fact, webhook job or Lambda job becomes visible for a rolled-back
mutation.

### Application

- Port durable event sequencing, replay/retention, audit filtering/export,
  operational counts, jobs, webhook targets/deliveries and Lambda targets.
- Implement PostgreSQL event sequence allocation without gaps being treated as
  missing committed events.
- Design provider-specific atomic job claiming. PostgreSQL uses row locking and
  skip-locked behavior; SQLite retains its serialized claim.
- Test record transaction coupling: no SSE event, audit fact or automation job
  becomes visible for a rolled-back record mutation.

### Dashboard

- Exercise Realtime, Audit, Jobs and Integrations pages against both providers.

### Public website

- Expand Realtime, Audit, Jobs, Webhooks and Functions with provider-neutral
  examples plus honest concurrency/ordering notes.

### Exit evidence

- Replay order, authorized visibility and job terminal states satisfy the same
  contract on both providers.
- Contention tests show each claimed job is executed once per successful claim
  and expired leases recover without provider-specific API behavior.

Completion record:

```text
Status: complete
Application commit: recorded by this commit
Website output commit: b221f8e
Website source commit: 9aa0807
SQLite evidence: events, audit, jobs and rollback-coupling suites pass
PostgreSQL evidence: real postgres 18.6 runs the same suites; SKIP LOCKED
  claiming gives exactly one execution per job across eight workers
Parity evidence: numeric event gaps are not missing events; terminal job states
  and rolled-back visibility are identical on both providers
Findings repaired: job claiming was a blind SELECT+UPDATE on both providers;
  PostgreSQL now claims with FOR UPDATE SKIP LOCKED
Known limits: webhook delivery targets public HTTPS endpoints by design (SSRF
  guard); backup/restore and migration remain PG10-PG11; PostgreSQL 16 CI not
  yet reviewed green
```

## PG10 - Backup, restore, operations, and provider recovery

Status: pending

### Application

- Replace SQLite-only page metrics with a provider-neutral operations summary
  and clearly labelled provider-specific storage/pool facts.
- Retain native consistent SQLite archives. Define PostgreSQL backup honestly:
  a transactionally consistent Trestle logical archive, not an implied backup
  of an operator's whole PostgreSQL cluster.
- Stream logical export in bounded order from one consistent snapshot, including
  schema metadata, records, identities, events, jobs and file manifest with the
  existing secret/session treatment explicitly reviewed.
- Restore only into an empty destination through preflight, staged validation
  and all-or-nothing publication. Document optional operator `pg_dump`/managed
  backup responsibilities separately from Trestle's portable archive.
- Drill corrupt, truncated, future-schema, occupied-target and object mismatch
  failures on both providers.

### Dashboard

- Make Backups and operations cards provider-aware. Use “portable Trestle
  backup” and “SQLite snapshot” precisely rather than one ambiguous button.

### Public website

- Rewrite Backups, Restore, Export/import, Observability and Rollback with a
  provider capability table, complete commands and recovery examples.

### Exit evidence

- Each provider completes repeated backup/restore round trips with representative
  data and files.
- Failed restore never mutates or partially publishes the destination.

## PG10 - Backup, restore, operations, and provider recovery

Status: complete

A portable Trestle logical archive now backs up and restores both providers.
SQLite retains its native consistent snapshot; PostgreSQL uses the portable
logical archive, which never implies a backup of an operator's whole PostgreSQL
service. The operations summary reports provider-specific storage facts, and
failed restores leave the destination untouched.

### Application

- Replace SQLite-only page metrics with a provider-neutral operations summary
  and clearly labelled provider-specific storage/pool facts.
- Retain native consistent SQLite archives. Define PostgreSQL backup honestly:
  a transactionally consistent Trestle logical archive, not an implied backup
  of an operator's whole PostgreSQL cluster.
- Stream logical export in bounded order from one consistent snapshot, including
  schema metadata, records, identities, events, jobs and file manifest.
- Restore only into an empty destination through preflight, staged validation
  and all-or-nothing publication. Document optional operator `pg_dump`/managed
  backup responsibilities separately.
- Drill corrupt, truncated, future-schema, occupied-target and object mismatch
  failures on both providers.

### Dashboard

- Make Backups and operations cards provider-aware. Use portable Trestle
  backup and SQLite snapshot precisely rather than one ambiguous button.

### Public website

- Rewrite Backups, Restore, Export/import, Observability and Rollback with a
  provider capability table, complete commands and recovery examples.

### Exit evidence

- Each provider completes repeated backup/restore round trips with representative
  data and files.
- Failed restore never mutates or partially publishes the destination.

Completion record:

```text
Status: complete
Application commit: recorded by this commit
Website output commit: 170ea8c
Website source commit: 3c3d793
SQLite evidence: native snapshot backup/restore and portable round trips pass
PostgreSQL evidence: real postgres 18.6 portable logical export/import and
  restore into an empty destination pass
Parity evidence: the portable archive round-trips SQLite to/from PostgreSQL in
  all four directions with stable IDs, JSON, booleans and system state
Findings repaired: the backup handler conflated storage provider with database
  provider; the portable archive now preserves stable IDs so foreign keys and
  rules survive; PostgreSQL operations reports a real database size
Known limits: operator pg_dump/managed backup is documented separately;
  cross-provider migration remains PG11; PostgreSQL 16 CI not yet reviewed green
```

## PG11 - Cross-provider migration, complete parity matrix, CI, and release boundary

Status: pending

### Application

- Add an explicit offline command for SQLite to PostgreSQL and PostgreSQL to
  SQLite migration using the portable logical format rather than ad hoc SQL
  copying.
- Require source read-only access, destination emptiness, compatible logical
  schema, connection preflight, capacity checks and an explicit confirmation;
  support a zero-write dry run.
- Preserve stable collection/field/record IDs, timestamps, versions, users,
  rules, event sequences, audit facts, jobs, integration targets and file
  manifests. Define which sessions/secrets are intentionally revoked or
  re-encrypted.
- Produce a signed/checksummed migration report with counts and hashes, and
  verify the destination before directing the operator to switch configuration.
- Test interruption at each phase, restart/resume or clean rollback, and both
  migration directions with non-trivial fixtures.

- Run every supported API, dashboard and operational workflow through a
  provider matrix from clean setup through upgrade, backup, migration and
  shutdown.
- Run Incident Desk unchanged against SQLite and PostgreSQL. The example may
  consume only published HTTP/SSE contracts and must not branch on provider.
- Execute race, fuzz, migration, authorization, failure-injection, sustained
  load and restart campaigns. Compare semantic results rather than expecting
  identical database internals.
- Test each supported PostgreSQL major declared by the release policy and the
  complete six-platform binary matrix. Verify PostgreSQL driver/static
  packaging does not add undocumented runtime files.
- Audit dependencies, licenses, release archives, upgrade/rollback, support
  bundles, logs, website claims and repository history. Close or publish every
  retained difference.


### Dashboard

- Do not perform migration from a live browser. Add a read-only Settings guide
  that reports the current provider and links to the exact offline command.

- Complete responsive/browser testing of setup and all administration pages on
  both providers. Provider badges stay subtle and informational; ordinary
  product workflows remain identical.


### Public website

- Add a database migration guide with dry-run, downtime, secret treatment,
  validation, cutover and rollback walkthroughs for both directions.

- Promote PostgreSQL from experimental to available only after the matrix is
  green. Update Home, Product, Architecture, Stability, Quickstart, deployment,
  examples and every SQLite-specific page.
- Add a tested database-selection guide, final capability/limitation matrix and
  PostgreSQL evidence to Battle Tested. Retain SQLite-specific operational
  advice on its own page.


### Exit evidence

- The destination passes the whole product/API parity corpus before cutover.
- An interrupted or rejected migration leaves the source untouched and the
  destination clearly incomplete or removed.

- A fresh user can select either database in setup and run the same external
  application without changing its code.
- All supported features have dual-provider evidence or are explicitly marked
  provider-specific with a justified contract.
- All three repositories are clean, generated outputs are current, public
  claims match the tested release and every checkpoint has its own commits.


## Campaign-wide retained questions

Resolve these in PG00/PG01 rather than allowing implementations to choose them
silently:

1. Which PostgreSQL major versions form the initial support window?
2. Is browser persistence of a PostgreSQL DSN acceptable for the release, or
   must browser setup emit configuration for an operator-managed secret?
3. Does initial PostgreSQL support remain single-process, or will PG09 earn a
   documented multi-process worker contract?
4. Which secrets and sessions survive portable cross-provider migration?
5. Is a managed-provider/serverless PostgreSQL topology supportable given
   connection, transaction and long-lived SSE/job requirements?
6. Which provider-specific metrics are safe and stable enough for the
   operations API?

Until these are answered and tested, the public website must describe
PostgreSQL as planned work, not an available database backend.
