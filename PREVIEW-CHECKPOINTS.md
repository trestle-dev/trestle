# Trestle public-preview campaign

This campaign prepares Trestle for an honest public preview. It is distinct
from the earlier implementation campaign (`CHECKPOINTS.md`, CP00-CP23) and the
PostgreSQL parity campaign (`POSTGRES-CHECKPOINTS.md`, PG00-PG12). Those
campaigns are complete; this one hardens the whole product, re-proves
SQLite/PostgreSQL parity with real-service evidence, establishes reproducible
releases and installers, connects the public domain and documentation, and ends
in a strict publish/no-publish review.

Checkpoints here are named CP1-CP22 in the order of the preview plan. They are
deliberately distinct from the historical CP00-CP23 numbering. Phase ordering
and safety gates are preserved from the plan. Every checkpoint ends in one
focused application commit plus one website commit whenever public
documentation changes; both hashes are reported against the checkpoint.

## Positioning

> Trestle is a compact, open-source backend platform you run yourself. Build
> with collections, authentication, files, realtime events, webhooks, jobs and
> generated APIs. Start with SQLite or use PostgreSQL when your deployment
> needs it.

The preview must not claim that Trestle is battle-proven, highly available, a
drop-in Supabase replacement, or inherently immune to outages merely because it
is self-hosted.

> Trestle is an early preview suitable for evaluation, prototypes, internal
> tools and developers willing to test recovery carefully. It is not yet
> presented as a battle-proven replacement for mature production platforms.

Self-hosting gives operators control of their failure domain, data, upgrades
and recovery. It does not guarantee uptime.

## Working rules

- One focused application commit per checkpoint; one corresponding website
  commit when public documentation changes. Report both hashes.
- Keep the application README and this handover current.
- Edit canonical Nift source only (`content/`). Rebuild generated output with
  Nift. Never hand-edit generated pages as the source of truth.
- Commit discipline for the website repository (see the model section):
  generated `public/` output commit first, then website source with its updated
  gitlink, when both change. This is the repository's documented convention.
- Add executable regression evidence for every fixed defect or new guarantee.
- Keep SQLite and PostgreSQL behaviour explicitly distinguished.
- Do not silently weaken tests to make a checkpoint pass.
- Do not push, tag, publish releases, alter DNS or publicise Trestle until
  explicitly authorized at the relevant gate.
- Stop immediately for any unresolved authentication bypass, authorization
  bypass, data-loss risk, migration corruption, unsafe secret handling or
  unreliable restore.
- Use human verification only where automation cannot reasonably establish the
  result. Label evidence accurately as automated, simulated, local real-service,
  CI, or human visual review.

## Repository identification and model

The Trestle workspace has two top-level repositories. The generated public
website is a nested Git repository inside the website repository, tracked by
the website source through a gitlink. This is the actual arrangement recorded
from repository evidence, not an assumed one.

| Role | Path | Branch | Head (recorded at campaign start) | Remote |
| --- | --- | --- | --- | --- |
| Trestle application | `trestle` | `main` | `974e3b3da0d72d3c8443031e1080c3053aa434c1` | `git@github.com:trestle-dev/trestle.git` |
| Nift website source | `trestle-dev.github.io` | `stage` | `e9d26efd175eccf8336248a93487c147577c7ac6` | `git@github.com:trestle-dev/trestle-dev.github.io.git` |
| Generated website (nested) | `trestle-dev.github.io/public` | `main` | `da053c21baa62238453e8d08ab81319423b228cb` | `git@github.com:trestle-dev/trestle-dev.github.io.git` |

Structure:

```text
trestle/
  content/              canonical dashboard source (Nift)
  internal/web/public/  generated embedded dashboard (Nift output, embedded in the binary)

trestle-dev.github.io/            (branch stage = website source)
  content/              canonical public-site source (Nift)
  templates/, scripts/  site templates and checks
  public/               generated public site = nested Git repo (branch main,
                        same remote), the GitHub Pages deployment target
```

Nested-repository evidence: the website source tracks `public` as a gitlink
(mode 160000), introduced at commit `6ef9bce` ("Plan Trestle public website",
2026-08-26) and documented by the website repo README ("Generated output is
stored in the nested `public/` Git repository... commit `public/` first and this
source repository second"). This is therefore NOT the Watchpost arrangement of a
separate top-level generated-site repository, but it does use a nested
generated-site repository; a single website product change is recorded as one
nested `public/` commit plus one website-source commit that bumps the gitlink.
No artificial repositories are manufactured and generated output is never
treated as canonical source.

A recorded external event: on 2026-08-30 the nested `public/` working tree was
replaced by a fresh clone of `origin/main`, which had advanced with commit
`80c06a7` "Create CNAME" (the `trestle.cv` domain CNAME). The website source
repository was reset to `e9d26ef`, discarding earlier local-only website
commits. The source gitlink was subsequently reconciled to the real nested head
`80c06a7` in this campaign's baseline.

## Nift tool record

The exact Nift executable used to build both sites:

| Item | Value |
| --- | --- |
| Path | `/usr/local/bin/nift` |
| Version | `Nift v4.0.8` |
| Dashboard build | `nift build` in `trestle/` (`content/` → `internal/web/public/`) |
| Public site build | `nift build` in `trestle-dev.github.io/` (`content/` → `public/`) |
| Status command | `nift status` (reports tracked pages up to date) |

`/home/nick/Repositories/nift/nift` is the Nift source project, not a Trestle
repository and not a built binary. The installed binary was present before this
campaign and is used unchanged. Clean `nift status`/`nift build` runs at
baseline produced no unexplained diffs in either repository.

`.nift/.ownership-gate` provenance: a zero-byte, 0644, never-removed flock
serialization sentinel created on demand by `nift build`
(`ProjectOwnership::acquire()`, Nift CP15 fix; `src/ProjectOwnership.cpp`). It
is build infrastructure, recreated as needed, and gitignored in the canonical
Nift ecosystem (`nift-embed-regression-suite/.gitignore`). The application
repository's copy was created by this campaign's own `nift build` and removed;
the website source's copy predates this campaign (2026-08-29) and is kept on
disk but gitignored. Neither is user content; no process outside Nift's build
ownership protocol depends on it.

## Domain record

The intended public domain is **`trestle.cv`**. Use exactly this domain in
future website, release and installer preparation:

```text
https://trestle.cv
https://trestle.cv/docs
https://trestle.cv/install.sh
```

Installer examples are `curl -fsSL https://trestle.cv/install.sh | sh` and
`curl -fsSL https://trestle.cv/install.sh | sudo sh -s -- --system`. A CNAME
for `trestle.cv` already exists in the generated site (`public` head `80c06a7`,
pushed to `origin/main`). DNS, hosting, canonical URLs and live external
configuration are NOT changed by this campaign without explicit authorization.

## Plan-vs-implementation reconciliation

The plan was written as if PostgreSQL work had not started. Repository evidence
differs, and these differences are recorded before CP1.

1. **PostgreSQL parity already exists.** `POSTGRES-CHECKPOINTS.md` records a
   complete PG00-PG12 campaign: real-server provider-parameterized parity,
   offline cross-provider migration, portable backup/restore, and a promoted
   16/17/18 support window. CP1's baseline is therefore a verification of an
   advanced baseline, not a first port. The baseline was measured at the
   original application head and was green on both providers.
2. **Existing commit messages are not evidence.** Every PG claim that this
   campaign depends on is re-run against real PostgreSQL services where the
   environment permits (local real-service PostgreSQL 18) or is labelled CI
   evidence from the historical matrix where it is not re-runnable (16/17).
3. **Checkpoint mapping and classification.** The prior PG checkpoints are
   mapped onto CP1-CP8 in the table below with each acceptance item classified.
4. **Local PostgreSQL versions.** Only PostgreSQL 18 is installed locally. This
   campaign provisions disposable real instances (`initdb`/`pg_ctl`) for
   reproducibility and exercises PostgreSQL 18.6; 16/17 rest on the historical
   CI matrix and are not re-claimed here.
5. **Two top-level repositories with a nested generated-site repo** (model
   above). This differs from the plan's assumption of a separate generated
   website repository and from the user's stated two-repository model; recorded
   with evidence so the campaign uses the actual arrangement.
6. **Incident Desk is a separate repository** (`trestle-example`); the earlier
   campaign exercised it via the published HTTP/SSE contract corpus. CP14
   rebuilds it from a clean checkout where the environment permits.

## PostgreSQL checkpoint mapping (prior work to new plan)

| New checkpoint | Prior PG work | Classification |
| --- | --- | --- |
| CP1 Contract and baseline | PG00 inventory, PG01 config/secrets, PG03R diagnostics, PG12 support window | already implemented; consolidated into a machine-readable readiness contract, contract test and reproducible gate; baseline independently re-run |
| CP2 First-run setup | PG01/PG03/PG03R/PG04 first-run state machine and dashboard truthfulness | mostly implemented; verify every acceptance transition and add state-machine regression tests where missing |
| CP3 Schema and migration integrity | PG03 dual migrations, PG03R history authority, PG11R2 complete history validation | largely implemented; audit and consolidate evidence; add any missing locking/traffic-gating assertions |
| CP4 Transactional mutation and audit atomicity | PG06 batch/rollback, PG09 composed rollback (records+events+audit+webhooks+lambda) | partially implemented; full inventory of security-sensitive mutations with failure injection between statements |
| CP5 Concurrency, locking, conflicts | PG04 single-winner setup, PG06 one-winner races, PG09 SKIP LOCKED claiming, PG03 concurrent startup | partially implemented; extend to the full race matrix including delete-vs-update, revocation-during-request, schema-change-during-access |
| CP6 Connection loss and pool recovery | PG03R connect timeout, readiness gating, pool bounds | mostly absent; new hardening with unavailable/degraded/recovered states and liveness semantics |
| CP7 Backup, restore, DR | PG10 portable archive, PG11R snapshot/confirmation/read-only source, PG11R2 hostile-archive preflight, PG11R3 empty destination | largely implemented; verify and consolidate evidence |
| CP8 Longevity and resource bounds | existing `test-load.sh`, `stress-concurrency.sh` | partially implemented; add a bounded soak with resource-growth tracking and a longer external package |

## Baseline evidence (measured before CP1 changes)

Measured at the original application head `974e3b3` (pre-change worktree) and on
the current tree before this campaign's CP1 consolidation:

| Gate | Result | Evidence type |
| --- | --- | --- |
| `go test -count=1 ./...` (SQLite, at `974e3b3`) | green, exit 0 | automated |
| `go test -count=1 ./...` (real PostgreSQL 18.6) | green, exit 0 | local real-service |
| `go test -race -count=1 ./...` (SQLite) | green, exit 0 | automated |
| `go test -race -count=1 ./...` (real PostgreSQL 18.6) | green, exit 0 | local real-service |
| `go vet ./...` | green, exit 0 | automated |
| `go build ./...`, `go build ./cmd/trestle` | green, exit 0 | automated |
| `./scripts/test-release-candidate.sh` (with real PG) | green, exit 0 | automated + local real-service |
| Frontend JS syntax + dashboard quality checks | green | automated |
| Website `nift status`/`nift build` | all pages up to date, no unexplained diff | automated |
| Website link/structure `scripts/check-site.mjs` | 87 HTML pages and 93 files checked | automated |
| `git status --porcelain` all three repos | empty | automated |
| `git fsck --full` | pending at CP1 record | automated |

Baseline failures recorded: none. PostgreSQL versions genuinely exercised
locally: **18.6** (disposable real instance provisioned with `initdb`/`pg_ctl`
and the pre-existing system `postgres:18` cluster; the system cluster's
`postgres` role is not usable for tests). PostgreSQL 16 and 17 are not installed
locally; their evidence is the historical CI matrix and is labelled CI.

## CP1 - Freeze the PostgreSQL contract and baseline

Status: complete

### Application

- Add a machine-readable PostgreSQL readiness contract at
  `docs/postgres/postgres-readiness.json` covering supported versions,
  configuration inputs, connection and TLS expectations, schema ownership,
  migration behaviour, transaction boundaries, pooling, timeouts, cancellation,
  retries, backup and restore, supported and unsupported deployment shapes, and
  differences from SQLite.
- Add `TestPostgresReadinessContract` in `internal/store` that enforces the
  contract against the compiled behaviour and, when `TRESTLE_TEST_POSTGRES_URL`
  is set, probes a real server and asserts its major version is inside the
  declared window.
- Add `scripts/test-postgres-gate.sh`, the one reproducible PostgreSQL-focused
  gate: real server (provided or disposable `initdb`/`pg_ctl` instance), never
  prints credentials, runs the contract probe, the full provider-parameterized
  suite, and the race-enabled provider suite.
- Correct stale project documentation discovered during reconciliation:
  HANDOVER "product-definition/scaffold stage" and "SQLite foundation", PLAN
  "PostgreSQL deliberately deferred", STABILITY single-SQLite wording, and the
  README node-check path. The project is described as pre-release hardening in
  preparation for an honest public preview, not as battle-proven or
  production-ready.
- Record the corrected repository model, Nift tool, `trestle.cv` domain
  decision, PG checkpoint mapping, and measured baseline in this ledger.

### Exit evidence

- Real PostgreSQL is used, never a mock: the gate provisions or connects to a
  real server and the contract test asserts the running major version.
- Existing behaviour was measured before repair: baseline table above (green at
  `974e3b3` and on the current tree); no pre-existing failure is hidden.
- Unsupported behaviour is documented: contract lists unsupported deployment
  shapes and the differences from SQLite.

Completion record:

```text
Status: complete
Application commit: 04b6647126251bf8bea684b2878fe92c664e2191
Website output commit: 80c06a7 (reconciled nested head; CNAME) / CP1 site commit recorded on rebuild
Website source commit: c686afd (baseline reconciliation) / CP1 site commit recorded on rebuild
Baseline evidence: green on SQLite and real PostgreSQL 18.6 at 974e3b3 before changes
PostgreSQL evidence: disposable real server 18.6; contract probe; full suite; race suite
Findings repaired: readiness contract was prose-only; no single reproducible PostgreSQL gate existed; stale product-stage documentation; stale README check path
Known limits: local evidence covers PostgreSQL 18; 16/17 rest on the historical CI matrix
```

## CP2 - Repair first-run PostgreSQL setup (ordinary-user workflow)

Status: complete

The full ordinary-user workflow was exercised against real PostgreSQL 18.6
(fresh instance -> choose PostgreSQL -> test connection -> persist configuration
-> prominent restart instructions -> restart -> create the first administrator
-> sign in) both as a real-process shell drill and as a provider-parameterized
Go workflow test. No product defect was found in the workflow itself; the
checkpoint therefore adds the executable state-machine regression coverage the
contract requires and locks the transitions.

### Application

- Extract the first-run database-selection state machine into a pure, DOM-free
  module (`content/assets/js/database-setup-state.js`) and route
  `database-setup.js` and the auth-gate copy through it, so every transition is
  testable without a browser. Behavior is unchanged:
  - an empty PostgreSQL URL leaves the test/apply button visibly and
    functionally disabled (CSS `button:disabled` opacity/cursor, asserted by the
    dashboard quality gate);
  - a non-empty URL enables it, with enabled hover styling applied only to
    enabled buttons;
  - invalid URLs produce useful, non-secret-leaking errors (TLS, unreachable,
    timeout envelopes);
  - a successful test shows a prominent "Restart required" notice;
  - after restart the interface says the user is creating the first
    administrator (single-sourced copy), never a sign-in form, and the two are
    never shown simultaneously (single auth form);
  - the seven-character password minimum is enforced (HTML `minlength="7"` and
    `adminauth.MinPasswordLength`).
- Add `scripts/test-database-setup.mjs`, a Node state-machine regression test
  covering every transition, wired into the browser-quality gate.
- Strengthen `scripts/check-dashboard-quality.mjs` with the structural contract
  (one auth form, one submit button, password minimum, present apply button,
  disabled vs enabled hover styling).
- Add `internal/databasesetup/workflow_test.go`, a provider-parameterized
  first-run lifecycle test (SQLite always, real PostgreSQL when configured):
  fresh selectability, persistence with 0600 permissions and atomic publication
  (no temporary file left), fixed provider after restart, password minimum,
  setup closure after the first administrator, and no connection-material
  leakage in any response.

### Exit evidence

- State-machine and workflow tests pass on SQLite and real PostgreSQL 18.6.
- The real-process shell drill (selection, persist, restart, first admin,
  sign-in) completes; logs contain no connection material.

Completion record:

```text
Status: complete
Application commit: 4c7534f
Website output commit: n/a (no public documentation change)
Website source commit: n/a (no public documentation change)
Verified: go test ./... (sqlite + real PostgreSQL 18.6); browser-quality gate
  including the new state-machine test; go vet; gofmt
Findings repaired: none (workflow already correct); added missing executable
  regression coverage for every first-run transition
Known limits: browser-level coverage is via the pure state machine and DOM
  structural checks, not a full browser harness; real-process drill is a shell
  simulation of the browser transitions
```

## CP3 - PostgreSQL schema and migration integrity

Status: complete

Audited every schema-creation and migration operation against the CP3 contract
and closed the concrete evidence gaps. The pre-existing store suite already
proved clean bootstrap, failed-migration rollback, future-schema refusal,
history reconciliation and concurrent-startup locking on both providers; the
checkpoint added what was genuinely missing and actionable recovery guidance.

### Application

- Add recovery instructions to fail-closed migration errors (gapped, future,
  misnamed or marker-disagreement history now directs the operator to restore
  the database from a backup before retrying), on both providers.
- Add `TestUpgradeFromEveryRetainedSchemaVersionProvider`: upgrades from every
  retained logical schema version (v0..v13) on SQLite and real PostgreSQL,
  building historical fixtures from the authoritative per-version DDL, proving
  the applied version reaches `CurrentVersion`, exactly `CurrentVersion`
  migration rows (no re-application), and prior data survives.
- Add `TestBootstrapSchemaIntegrity`: introspects a fresh bootstrap on both
  providers and asserts the exact system-table set, cascade foreign keys
  (admin sessions->admins, fields->collections, collection rules->collections),
  uniqueness (fields collection_id+name, token_hash, refresh_hash) and the
  named operational indexes.
- Add `TestMigrationFailuresCarryRecoveryInstructions`: gapped history fails
  closed with the actionable recovery hint on both providers.

Traffic gating is by construction: the store opens (and migrations complete)
before the HTTP server begins serving, and `/system/ready` reports not-ready
until `SetReady(true)` after startup.

### Public website

- Migrations page now states that a rejected database leads Trestle to direct
  the operator to restore from a backup before retrying.

### Exit evidence

- Upgrade-from-every-version, schema-integrity and recovery-instruction tests
  pass on SQLite and real PostgreSQL 18.6; full suite and vet green.

Completion record:

```text
Status: complete
Application commit: de37183
Website output commit: c11608e
Website source commit: 50bc788
Verified: store suite, full go test ./... and go vet on SQLite and real
  PostgreSQL 18.6; Nift rebuild and site check
Findings repaired: PostgreSQL had no persistent upgrade-from-every-version
  executable test; no schema-introspection test asserted indexes/foreign
  keys/uniqueness; fail-closed migration errors lacked an operator recovery step
Known limits: fixtures model each retained logical version with the current
  authoritative DDL; historical DDL as first written is not retained separately
```

## CP3R - Freeze the retained migration lineage

Status: complete

CP3's upgrade fixtures were generated from the same current migration
definitions being validated, so a later edit to an old migration could silently
redefine what historical schema version vN means. This repair freezes the
retained lineage independently before public preview.

### Application

- Add an append-only migration lineage manifest
  (`internal/store/migrations_manifest.json`) with, for every retained version:
  version, migration name, SHA-256 of the normalized SQLite DDL, and SHA-256 of
  the normalized PostgreSQL DDL. The normalization (strip CR, trim lines, drop
  blanks, join with single newline) is recorded in the manifest so the digest is
  reproducible.
- Add `TestMigrationLineageFrozen`: requires versions contiguous 1..CurrentVersion,
  names and digests match the compiled migrations (editing or removing a
  retained migration fails), both SQLite and PostgreSQL definitions exist for
  every version, and `CurrentVersion` equals the manifest final version.
- Add `TestWriteMigrationLineageManifest` (generator, gated by
  `TRESTLE_LINEAGE_WRITE=1`) so appending a new migration can deliberately
  regenerate and extend the frozen baseline.

The lineage is described accurately as the frozen pre-preview migration
lineage, not as production upgrade fixtures from publicly released versions.

### Exit evidence

- Manifest generated at `CurrentVersion` 13; the frozen gate passes.

Completion record:

```text
Status: complete (repair)
Application commit: recorded by this commit
Website output commit: n/a (no public documentation change)
Website source commit: n/a (no public documentation change)
Verified: TestMigrationLineageFrozen passes; manifest lists 13 retained
  migrations with both digests
Findings repaired: old migrations could be edited after use without detection
Known limits: appending migration 14 (CP4R) will exercise the append-only path
  and regenerate the manifest
```

## CP4 - Transactional mutation and audit atomicity

Status: complete

Inventoried every security-sensitive multi-table mutation and proved each
commits atomically under PostgreSQL (or documents a deliberately non-atomic
external effect). The composed rollback test already covered records + events +
audit + webhook and Lambda outbox jobs; this checkpoint repaired the one
genuine non-atomicity found and locked it with failure injection between
statements.

### Application

- Defect repaired: application login and refresh-token rotation inserted the
  session row and its short-lived access token in two separate
  non-transactional statements. A failure between them left an orphaned session
  on login, and on refresh consumed the old refresh token without issuing an
  access token. Both now commit the session and access rows in one transaction
  (`createAccessTx`).
- Add `TestLoginAndRefreshAreAtomicUnderInjectedFailure`: a fault-injecting
  executor fails the access insert after the session row was written and proves,
  on SQLite and real PostgreSQL, that login leaves no session or access row and
  a failed refresh leaves the original refresh token valid.
- Add `docs/hardening/atomicity-inventory.json`: the machine-readable inventory
  of every security-sensitive multi-table mutation with its transaction
  boundary, evidence and deliberately non-atomic external effects, plus the
  remaining hardening items.

### Exit evidence

- Failure-injection atomicity test passes on SQLite and real PostgreSQL 18.6;
  full suite, race and vet green on both providers.

Completion record:

```text
Status: complete
Application commit: c8df51a
Website output commit: n/a (no public documentation change)
Website source commit: n/a (no public documentation change)
Verified: appauth atomicity test on SQLite and real PostgreSQL 18.6; full go
  test ./..., race and vet on both providers
Findings repaired: login/refresh session+access non-atomicity (orphaned
  sessions; refresh token consumed without an access token)
Known limits: file deletion was not yet fail-closed (repaired in CP4R); admin
  setup issues its session after the admin commit as a documented convenience
```

## CP4R - Durable, fail-closed file deletion

Status: complete

CP4's atomicity inventory flagged that permanent file deletion was fail-open:
the handler ignored every database error, deleted the stored object even when
the metadata delete failed, and could return 204 without durable deletion
state. This repair replaces it with a recoverable deletion state machine.

### Application

- Migration 14 "durable file deletion" (append-only; regenerated the lineage
  manifest to final version 14): adds `_trestle_file_deletions` (id, storage_key,
  status pending/done, attempts, created_at, finalized_at) and
  `_trestle_files.deleted_at`.
- `files.remove` now: (1) records durable intent and marks metadata unavailable
  in one transaction, (2) commits, (3) deletes the storage object, (4) finalizes
  the intent. A failed begin, intent write or commit returns an error and
  changes nothing; no object is deleted before durable intent exists; a storage
  failure returns `deletion_pending` and leaves the intent pending; success is
  returned only after the object is gone and the intent is finalized.
- `ResumePendingDeletions` recovers unfinished deletion at startup (and cleans
  files restored from an archive whose deletion was pending). Storage deletion
  and finalization are idempotent, so a crash at any point converges; the sweep
  never touches objects referenced by live metadata.
- Downloads and the file list refuse files once `deleted_at` is set.
- `_trestle_file_deletions` is added to the portable-owned tables so an import
  destination must be empty of deletion state.
- Failure-injection tests at every boundary on SQLite and real PostgreSQL:
  begin, intent-write, commit, storage-deletion failure + resume, crash between
  storage deletion and finalization + resume, retry/duplicate worker, and
  restored-deleted-file cleanup.
- `docs/hardening/atomicity-inventory.json` now describes the implemented state
  machine instead of retaining the known defect.

### Exit evidence

- Durable-deletion tests pass on SQLite and real PostgreSQL 18.6; full files,
  backup and store suites pass on both providers; vet clean.

Completion record:

```text
Status: complete (repair)
Application commit: recorded by this commit
Website output commit: n/a (no public documentation change)
Website source commit: n/a (no public documentation change)
Verified: durable-deletion suite on SQLite and real PostgreSQL 18.6; full
  files/backup/store suites; vet
Findings repaired: fail-open file deletion (ignored begin/metadata/commit
  errors; object deleted without durable intent; 204 without durable state)
Known limits: deletion state is retained as audit rows; a dedicated periodic
  sweep beyond startup is not implemented (startup and the documented resume
  path recover pending deletion)
```


## CP3R2 - Append-only migration lineage updater

Status: complete

CP3R's generator regenerated every historical digest from the current compiled
migrations, so someone could modify an old migration and run the generator to
silently bless the rewrite. This repair makes manifest updates genuinely
append-only.

### Application

- The updater (`TestUpdateMigrationLineageManifest`, gated by
  `TRESTLE_LINEAGE_WRITE=1`) now reads the committed manifest, validates every
  retained entry against the compiled migrations (version, name, SQLite digest,
  PostgreSQL digest), refuses to write when any retained entry differs, and
  appends entries only for versions after the existing `finalVersion`. It is a
  no-op when no new migration exists and refuses truncation, gaps, reordering,
  replacement or a manifest ahead of the compiled version.
- `appendOnlyUpdate` is the testable pure function behind the updater.
- `TestAppendOnlyUpdateRefusesHistoricalChanges` proves changed historical
  SQLite DDL, changed historical PostgreSQL DDL, renamed migrations, removal,
  reordering and truncation all fail, while adding migration 15 appends exactly
  one entry with entries 1-14 preserved field-for-field, and running with no new
  migration is a no-op.
- The normal development command is documented in HANDOVER.md: append the
  migration first, then run the append-only updater.

### Exit evidence

- Frozen gate and append-only updater tests pass; the updater run against the
  current manifest is a no-op.

Completion record:

```text
Status: complete (repair)
Application commit: recorded by this commit
Website output commit: n/a (no public documentation change)
Website source commit: n/a (no public documentation change)
Verified: TestMigrationLineageFrozen, TestAppendOnlyUpdateRefusesHistoricalChanges
  and the no-op updater run pass
Findings repaired: the manifest generator could bless a rewritten historical
  migration; it is now strictly append-only
Known limits: none retained
```


## CP4R2 - Fail-closed deletion recovery and periodic worker

Status: complete

CP4R's recovery sweep selected every pending intent, so an inconsistent
restored/imported state pairing a pending intent with live metadata
(deleted_at IS NULL) could delete an object still referenced by live metadata.
This repair makes recovery target selection fail closed and adds bounded
periodic recovery so convergence does not depend on a restart.

### Application

- `ResumePendingDeletions` now selects only pending intents whose file metadata
  is absent or marked deleted (LEFT JOIN guard) and, before each storage
  deletion, confirms no live `_trestle_files` row references that storage key
  (same id or any live file). A live reference causes the intent to be skipped,
  logged (SetLogger) and left pending, and it remains recoverable once the
  reference is deliberately resolved.
- Add `RunDeletionRecovery`: a bounded, shutdown-aware periodic worker
  (5-minute interval in the process) that resumes pending deletion during a
  continuously running process and logs observable status. The startup sweep is
  retained; the misleading 'cleanup endpoint' comment is corrected.
- Provider-parameterized tests (SQLite and real PostgreSQL): pending intent plus
  matching live metadata leaves the object intact; pending intent whose key is
  referenced by any live file leaves the object intact; pending intent with
  deleted metadata proceeds; pending intent with absent metadata proceeds; a
  conflicted intent stays recoverable after resolution; duplicate/concurrent
  recovery is harmless; the periodic worker finalizes during operation and stops
  on shutdown.

### Exit evidence

- All deletion and recovery tests pass on SQLite and real PostgreSQL 18.6.

Completion record:

```text
Status: complete (repair)
Application commit: recorded by this commit
Website output commit: recorded by this commit
Website source commit: recorded by this commit
Verified: deletion/recovery tests and full files suite on SQLite and real
  PostgreSQL 18.6; build and vet clean
Findings repaired: recovery could delete an object referenced by live metadata;
  convergence required a restart; misleading comment about a cleanup endpoint
Known limits: the recovery worker interval is a fixed 5 minutes; conflicts
  remain pending until the live reference is resolved
```


## CP5 - Concurrency, locking and conflict behaviour

Status: complete

Ran real PostgreSQL concurrency tests for the mutation races the plan lists
alongside race-enabled Go tests on both providers, and added the missing
executable corpus. Existing coverage already proved job claiming (eight workers,
one execution each), first-administrator single-winner, concurrent refresh and
logins with independent revocation, and concurrent-startup migration
serialization; this checkpoint adds the record, schema, import and revocation
races that had no persistent provider-parameterized tests.

### Application

- records: `TestConcurrentCreateUniqueFieldOneWinner` (simultaneous creation
  with a unique field resolves to exactly one winner), `TestConcurrentEditOptimisticVersionOneWinner`
  (optimistic-version update, exactly one winner, committed value is one of the
  two), `TestDeleteUpdateRaceResolvesConsistently` (delete-vs-update commits
  deterministically: update winner leaves the record at v2 with the delete
  rejected; delete winner leaves no record with the update rejected as 404/409/412),
  `TestSchemaChangeDuringRecordAccess` (a schema change racing record writes
  leaves a non-torn field set and never loses a 201-committed record; a clean
  retryable rejection is accepted as deterministic semantics).
- backup: `TestConcurrentImportSingleWinner` (two concurrent imports into an
  empty initialized destination: at most one succeeds and the destination is
  either the full archive or unchanged; no partial merge).
- appauth: `TestRevocationDuringInFlightRequests` (in-flight authenticated
  requests observe only pre- or post-revocation state, never torn, and every
  request after logout returns is rejected).
- Full race-enabled suite runs green on SQLite and real PostgreSQL.

### Exit evidence

- Concurrency corpus passes on SQLite and real PostgreSQL 18.6; full Go and
  full race suites green on both providers.

Completion record:

```text
Status: complete
Application commit: recorded by this commit
Website output commit: n/a (no public documentation change)
Website source commit: n/a (no public documentation change)
Verified: concurrency corpus on SQLite and real PostgreSQL 18.6; full suite and
  full race suite on both providers
Findings repaired: none (races resolved deterministically by optimistic
  concurrency and transaction isolation); added the missing executable corpus
Known limits: schema-change-during-access may reject cleanly (422/retryable) as
  a documented, deterministic outcome; lock-ordering behaviour across arbitrary
  topologies is not exhaustively exercised
```


## CP6 - Connection loss, restart and pool recovery

Status: complete

Hardened connection-loss, restart and pool-recovery behaviour against a real
PostgreSQL instance and added readiness semantics orchestration can use.
Requests fail cleanly and promptly across every failure mode; the dashboard and
APIs can distinguish an unavailable database from a ready one, and readiness
recovers automatically when connectivity returns.

### Application

- `/system/ready` now reflects live database availability: a ready process
  whose database probe fails returns 503 `database_unavailable` (degraded),
  while `/system/health` stays 200 (process liveness). When the database
  recovers, readiness returns 200. main wires the store Ping as the probe.
- Add `TestReadinessDistinguishesDatabaseUnavailable` proving the
  unavailable/degraded/recovered transitions.
- Store connection corpus (real PostgreSQL 18.6): startup against an
  unavailable database fails bounded and cleanly; DNS failure fails within the
  configured bound; `sslmode=verify-full` rejects a server without a verifiable
  certificate; wrong credentials fail cleanly without leaking the secret
  (skipped on trust-authentication servers, documented); a cancelled request
  surfaces immediately; a single-connection pool under concurrent load
  serializes without hanging; a reset connection errors promptly instead of
  hanging.
- Add `scripts/test-connection-recovery.sh`: a real-service drill that starts
  Trestle against a disposable PostgreSQL, stops the database (ready -> 503
  database_unavailable, health stays 200), restarts it (ready -> 200), and
  asserts no connection material in logs. Wired into CI syntax checks.

### Exit evidence

- Connection corpus passes on SQLite and real PostgreSQL 18.6; the real-service
  drill passes with a live stop/restart of the database.

Completion record:

```text
Status: complete
Application commit: recorded by this commit
Website output commit: n/a (no public documentation change)
Website source commit: n/a (no public documentation change)
Verified: connection corpus on both providers; connection-recovery drill
  against a real disposable PostgreSQL; full suite and vet
Findings repaired: /system/ready did not reflect live database availability
  (could report ready with the database down); now distinct from health
Known limits: the dashboard distinguishes ready/unavailable via the readiness
  probe; a distinct three-way degraded state is represented by ready=503
  database_unavailable with health=200; wrong-credential rejection is exercised
  only where the server enforces passwords (the trust-auth disposable suite
  server skips that scenario)
```


## CP7 - Backup, restore and disaster recovery

Status: complete

Established a documented, tested PostgreSQL recovery contract and added a
destructive drill that runs against isolated temporary data only. The portable
Trestle logical archive is the cross-provider backup format and is described
honestly: it never implies a backup of an operator's whole PostgreSQL cluster;
`pg_dump`/managed-cluster backup responsibilities are documented separately.

### Application

- Add `docs/postgres/recovery-contract.json`: the machine-readable recovery
  contract covering SQLite snapshot and portable archive formats, `pg_dump` as
  a separate operator responsibility, the restore contract (initialized,
  logically empty destination; all-or-nothing import; hostile-archive
  preflight; secrets/sessions treatment; file-manifest consistency), failure
  modes (corrupted/incomplete/future-schema/occupied/wrong-credentials/failed
  upgrade), semantic verification and operator steps.
- Add `scripts/test-restore-drill.sh`: a destructive backup, destroy, restore
  and verify drill against isolated temporary data. Leg 1 (SQLite): create
  data, online backup, destroy the data directory, offline restore into a new
  directory, sign in again (sessions are revoked by the restore policy) and
  verify the record. Leg 2 (PostgreSQL): restore the portable archive into an
  empty initialized destination and verify the record. Wired into CI syntax
  checks.
- Existing evidence consolidated: hostile-archive preflight, empty-destination
  rejection, snapshot consistency, session/secret revocation, cross-provider
  restore and concurrent-import single-winner tests already cover the contract
  on both providers.

### Exit evidence

- The destructive drill passes both legs against real disposable services
  (SQLite and PostgreSQL 18.6); full suite and vet green.

Completion record:

```text
Status: complete
Application commit: recorded by this commit
Website output commit: n/a (restore and backups pages already document the
  initialized-empty-destination and portable-archive contracts)
Website source commit: n/a
Verified: scripts/test-restore-drill.sh (both legs); full suite and vet
Findings repaired: none (restore semantics were already correct); added the
  machine-readable recovery contract and the destructive drill
Known limits: recovery-time and operator-step SLAs are documented behaviour,
  not measured guarantees; pg_dump remains an operator responsibility
```


## CP8 - Longevity and resource bounds

Status: complete

Ran a sustained workload with resource-growth tracking and packaged a longer
reproducible soak for external execution. The soak is an instrument for
regression and reproducibility, not a marketing benchmark: it asserts the
product does not leak resources, lose realtime events or error under sustained
load within the run, and reports numbers without claiming cross-machine
portability.

### Application

- Add `scripts/soak.sh` (`SOAK_SECONDS`, default 60): sustained CRUD with
  latency sampling, a realtime SSE subscriber counting delivered events, file
  upload, webhook-job enqueue (one per record; a saturated 200-cap list proves
  enqueue at scale while the Go claiming suite proves no duplication), online
  backup, a mid-soak restart, and /proc resource sampling (VmRSS, fd count,
  threads). Asserts zero API errors, realtime events delivered, no secrets in
  logs, and bounded resources.
- Defect repaired (found during the soak): the jobs-list API scanned
  `payload_json` directly into `*json.RawMessage`, which `database/sql` cannot
  do for a string driver value on this driver; every row's scan failed and the
  ignored error lost every job field except id and kind. The list handler now
  scans into a string and converts, checks scan errors, and
  `TestJobListFieldsRoundTrip` locks the fix on both providers.
- Evidence (this machine only): a 60s soak created 1,955 records with 0 API
  errors, observed realtime events, produced a 967 KiB backup, recovered across
  a restart, and showed stable resources (fd 9->9, threads 13->9, RSS falling
  after GC).

### Exit evidence

- Bounded soak passes locally; the script is packaged for longer external runs.
- Full suite and vet green on SQLite and real PostgreSQL 18.6.

Completion record:

```text
Status: complete
Application commit: recorded by this commit
Website output commit: n/a (no public documentation change)
Website source commit: n/a (no public documentation change)
Verified: soak runs (20s and 60s) with 0 API errors and stable resources; full
  suite and vet on both providers
Findings repaired: jobs-list API lost every field except id/kind because the
  scan into *json.RawMessage failed and the error was ignored
Known limits: webhook delivery to a private/loopback destination is refused by
  the SSRF guard, so the soak asserts webhook-job enqueue and relies on the Go
  claiming suite for drain/no-duplication; live HTTPS delivery requires a
  non-private endpoint and is exercised only in real deployments; resource
  numbers are this-machine-only
```


## CP3R3 - Close the truncated-manifest updater edge case

Status: complete

CP3R2's `appendOnlyUpdate` returned a no-op when
`existing.FinalVersion == currentVersion` before confirming the manifest was
internally consistent, so a truncated manifest (fewer entries than
`finalVersion`) paired with a matching `CurrentVersion` was treated as a no-op.
This repair validates manifest metadata and internal consistency before the
no-op branch.

### Application

- `appendOnlyUpdate` now validates, before any no-op: the lineage name and a
  supported `lineageVersion`; the recorded normalization contract; and
  `len(Migrations) == FinalVersion` with the last entry's version equal to
  `FinalVersion` (guarded so an empty manifest never indexes `[-1]`). Only a
  complete, internally consistent manifest at `CurrentVersion` is a genuine
  no-op; truncation, inconsistency, an empty manifest with a nonzero
  `finalVersion`, unsupported lineage versions and changed normalization
  metadata are all refused.
- `TestAppendOnlyUpdateManifestEdgeCases` covers: missing final entry with
  `finalVersion == CurrentVersion` is not a no-op; `finalVersion` below and
  above the last entry; empty manifest appending from scratch; empty manifest
  with nonzero `finalVersion` refused; unsupported lineage version; changed
  normalization; and a genuine no-op with a complete valid manifest.

### Exit evidence

- All append-only updater tests and the frozen-lineage gate pass; the updater
  run against the current manifest is a genuine no-op.

Completion record:

```text
Status: complete (repair)
Application commit: recorded by this commit
Website output commit: n/a (no public documentation change)
Website source commit: n/a (no public documentation change)
Verified: TestAppendOnlyUpdateManifestEdgeCases, TestMigrationLineageFrozen and
  the no-op updater run pass
Findings repaired: a truncated manifest could be mistaken for a no-op
Known limits: none retained
```


## CP5R - Repair concurrency evidence

Status: complete

CP5's revocation and import-race tests had evidence gaps: the revocation test
did not wait for the logout goroutine before asserting post-logout rejection,
and the import-race test checked only the collection count, which could not
distinguish a complete import from a partial one.

### Application

- `TestRevocationDuringInFlightRequests` now races K authenticated requests and
  the logout at one barrier, waits for both (logout in the wait group), captures
  and asserts the logout status is 204, then proves every subsequent
  authentication attempt is rejected. It runs 30 iterations with distinct users
  so both interleavings are exercised without requiring both in any single run,
  and requires at least one iteration where an in-flight request observed the
  pre-revocation state.
- `TestConcurrentImportSingleWinner` now compares a canonical semantic digest
  (`SemanticDigest`, covering every portable-owned table and physical record
  table) between the destination and the source archive / the pre-import
  destination: exactly one winner requires the destination digest to equal the
  source archive digest; zero winners require it to equal the pre-import digest.
  It also asserts the physical data-table count equals the collection count so
  no temporary or partially created physical tables remain.

### Exit evidence

- Both repaired tests pass on SQLite and real PostgreSQL 18.6; full suite and
  race green on both providers.

Completion record:

```text
Status: complete (repair)
Application commit: recorded by this commit
Website output commit: n/a (no public documentation change)
Website source commit: n/a (no public documentation change)
Verified: revocation race (30 iterations) and import semantic-digest tests on
  SQLite and real PostgreSQL 18.6; full suite and race
Findings repaired: revocation test raced its own assertion; import race proved
  only the collection count, not whole-destination equality
Known limits: none retained
```


## CP8R - Soak resource bounds and jobs-endpoint regression

Status: complete

CP8's soak sampled resources but never enforced a bound and compared a warm
baseline against a different restarted process, so it could not demonstrate
absence of growth; and the jobs-list regression test replayed the corrected
query manually instead of driving the real endpoint.

### Application

- `scripts/soak.sh` restructured:
  - warms the process before taking the baseline;
  - samples the SAME process throughout the sustained workload (baseline,
    periodic 5s samples, peak, final settled after a GC pause);
  - defines and enforces documented bounds: fd growth <= 25, thread growth
    <= 20, settled RSS growth <= 50 MiB, failing the script when exceeded;
  - moves restart recovery to a separate phase that verifies representative
    records, file metadata, backup visibility and persisted-session behavior,
    and is NOT used in the leak comparison;
  - narrows the webhook claim to enqueue activity only (a 200-item newest-first
    list saturated with webhook jobs), relying on the provider-parameterized Go
    jobs suite for claim/drain/no-duplication, and removes stale comments about
    stale sessions, duplicate deliveries and queue drainage.
- The jobs-list regression now drives the real authenticated `/admin/v1/jobs`
  endpoint: `TestJobsEndpointListsFullFieldsAndFilters` verifies valid JSON with
  payload, status, attempts, max attempts, timestamps and error/lease fields,
  multiple rows and status filtering on both providers;
  `TestJobsEndpointQueryFailureIsStructured` proves a list query/scan failure
  returns the normal structured API error envelope rather than a plain-text
  `http.Error` (the `list` handler now emits the structured envelope).
- Observed (this machine only, 60s default): fd growth 0, thread growth 1,
  settled RSS growth negative (GC returned memory) - all within the documented
  bounds.

### Exit evidence

- Soak passes with periodic sampling and enforced bounds at 25s and 60s; the
  jobs endpoint tests pass on SQLite and real PostgreSQL 18.6; full suite and
  race green; vet/gofmt clean.

Completion record:

```text
Status: complete (repair)
Application commit: recorded by this commit
Website output commit: n/a (no public documentation change)
Website source commit: n/a (no public documentation change)
Verified: soak at 25s and 60s with enforced bounds; jobs endpoint tests on both
  providers; full suite and race; release-candidate matrix; PostgreSQL gate;
  connection-recovery and restore drills
Findings repaired: soak did not enforce resource bounds and compared a warm
  baseline against a restarted process; jobs-list regression did not drive the
  real endpoint; jobs-list failures returned plain-text http.Error
Known limits: webhook delivery to a private destination is refused by the SSRF
  guard, so the soak asserts enqueue activity and the Go jobs suite covers
  claim/drain/no-duplication; resource numbers are this-machine-only
```


## CP5R2 - Narrow revocation-race evidence and harden the connection drill

Status: complete

CP5R's revocation test asserted that both in-flight interleavings were
exercised while only requiring a pre-revocation success in at least one
iteration; requiring the scheduler to also produce a post-revocation in-flight
rejection would be flaky in CI. This repair narrows the evidence to what is
actually guaranteed and hardens the connection drill's transient PostgreSQL
provisioning failure.

### Application

- `TestRevocationDuringInFlightRequests` now guarantees, per iteration:
  concurrent in-flight requests return a clean boolean (they may complete
  before or after the committed revocation, neither side required in any run);
  logout is synchronized in the same barrier and awaited with status 204; and
  every request started after logout returns is rejected. The probabilistic
  pre-revocation-success requirement is removed.
- The connection-loss and restore drills now print the PostgreSQL log and the
  chosen port on any `pg_ctl` start failure and retry provisioning on a fresh
  port (bounded), rather than silently normalising an infrastructure flake.

### Exit evidence

- Revocation race passes on SQLite and real PostgreSQL 18.6; both drills pass
  with the hardened provisioning.

Completion record:

```text
Status: complete (repair)
Application commit: recorded by this commit
Website output commit: n/a (no public documentation change)
Website source commit: n/a (no public documentation change)
Verified: revocation race; connection-loss and restore drills on a fresh
  disposable PostgreSQL
Findings repaired: revocation evidence claimed both in-flight interleavings
  without asserting post-revocation in-flight rejection; drills could fail on a
  transient PostgreSQL provisioning flake without diagnostics or retry
Known limits: in-flight requests may complete on either side of the committed
  revocation; only the guaranteed post-logout rejection is asserted
```


## CP9 - SQLite/PostgreSQL parity matrix

Status: complete

Built a durable, machine-readable parity inventory covering every supported
public and administrative API operation area. Parity is never marked from
shared handler source alone: a row is verified only when it names
provider-parameterized tests that run on SQLite always and on real PostgreSQL
when configured.

### Application

- Add `docs/postgres/parity-matrix.json`: a machine-readable inventory with
  statuses (verified / providerSpecific / unsupported / unverified), the
  implementation kind (shared handler vs provider-specific), provider-
  parameterized test evidence, and documented intentional differences across:
  collection schema creation/alteration/deletion and every field type and
  constraint; record CRUD and optimistic concurrency; filtering, sorting,
  nulls, pagination and cursor stability; batches and idempotency; users,
  credentials and access rules; files and quota; jobs, events and realtime;
  webhooks and functions; backup/export/import/restore/migration; audit and
  diagnostics; error codes and structured envelopes. Unsupported or deferred
  rows are listed explicitly (S3 storage is storage-provider-specific, managed/
  serverless PostgreSQL and multi-process clustering are unsupported).
- Add `TestParityMatrixEvidenceCrossCheck`: validates the inventory (name,
  version, statuses) and proves every verified row names at least one test that
  exists and whose file is provider-parameterized (iterates `storetest.Providers`
  or is gated on real PostgreSQL via `postgresTestURL`/`ownedURL`), so parity
  rows are always backed by executable evidence.
- Parameterize `TestProviderDiagnosticsDoNotExposeConnectionMaterial` across
  SQLite and real PostgreSQL so the diagnostics parity row has dual-provider
  evidence.

### Public website and handover

- Database support page notes the machine-readable parity matrix and the rule
  that verified parity requires provider-parameterized test evidence.

### Exit evidence

- Parity cross-check passes; the full provider-parameterized suite (the
  executable gate) is green on SQLite and real PostgreSQL 18.6.

Completion record:

```text
Status: complete
Application commit: recorded by this commit
Website output commit: recorded by this commit
Website source commit: recorded by this commit
Verified: TestParityMatrixEvidenceCrossCheck; full suite on both providers
Findings repaired: provider diagnostics evidence was SQLite-only; now dual-provider
Known limits: rows marked unsupported/unverified are explicit; the matrix is
  evidence-linked, not a claim of engine-internal identity
```


## CP10 - Authentication, authorization and session hardening

Status: complete

Conducted an adversarial review of administrator and application
authentication, explicitly inventoried ignored database errors, and repaired
every security-sensitive mutation so it never reports success when its durable
write failed. Failure injection on SQLite and real PostgreSQL proves the
fail-closed behaviour.

### Application

- Repaired ignored durable-write errors in security-sensitive mutations:
  administrator logout and application logout revocation (now return 500 on
  failure and leave the session valid); application user disable (now
  transactional: disabling the user and revoking its sessions commit together);
  webhook and function enable/disable (return 500/404 and leave the enabled
  state unchanged); credential revocation (checks the write error, not just
  affected rows); job cancel/retry (return 500 on failure); collection rule
  updates (a failed insert rolls back the delete and preserves the prior
  rules). Best-effort telemetry paths (credential last-used, audit facts) and
  the job worker's internal transitions are documented as such (the latter is
  recovered by lease expiry and idempotent delivery keys).
- Add `internal/authaudit/mutation_failure_test.go`:
  `TestSecuritySensitiveMutationsFailClosed` injects a durable-write failure
  into each of the seven paths and asserts a 500 and unchanged prior state on
  both providers.
- `docs/hardening/atomicity-inventory.json` gains an
  `ignoredDatabaseErrorsAudit` section listing the checked paths, the
  best-effort exceptions and the evidence.
- SECURITY.md documents the fail-closed durable-mutation guarantee.

### Public website and handover

- The security page documents the fail-closed durable-mutation guarantee.

### Exit evidence

- Failure-injection suite passes on SQLite and real PostgreSQL 18.6; full suite
  and vet green on both providers.

Completion record:

```text
Status: complete
Application commit: recorded by this commit
Website output commit: recorded by this commit
Website source commit: recorded by this commit
Verified: TestSecuritySensitiveMutationsFailClosed on both providers; full suite and vet
Findings repaired: logout/revocation/enable-disable/job-action/rule-update
  paths could report success when their durable write failed; disable-user was
  non-transactional
Known limits: best-effort telemetry and worker-internal transitions are
  documented; rate limiting and forwarded-proxy behaviour are covered by
  existing suites
```


## CP11 - Subsystem and integration hardening

Status: complete

Adversarially exercised the major subsystems and added regressions for the
gaps found. External side effects remain fail-closed and idempotent; the
existing suites already covered file deletion fail-closed recovery, webhook
SSRF/private-destination rejection and redirect refusal, snapshot consistency
under concurrent writes, and startup recovery workers.

### Application

- Webhook signing: extract `signWebhook` (SHA-256 HMAC over `timestamp.body`,
  `v1=` prefixed) and add `TestWebhookSignatureScheme` proving the value is
  reproducible by a receiver and changes under any secret, body or timestamp
  tampering.
- Jobs: `TestJobRetryBackoffThenDeadLetter` drives a failing job through
  retries with a future `available_at` (exponential backoff) to dead-letter
  after `max_attempts`, retaining the failure message, on both providers.
- Server: `TestGracefulShutdownDrainsInFlightRequest` proves Shutdown lets an
  in-flight request complete and returns within the drain deadline.
- Realtime: `TestSlowConsumerDoesNotBlockOtherSubscribers` proves a stalled SSE
  consumer blocks only its own connection (per-connection goroutine, bounded
  100-event batches); a healthy subscriber still receives committed events.

### Exit evidence

- New regression tests pass under race on SQLite and real PostgreSQL 18.6;
  full suite and vet green.

Completion record:

```text
Status: complete
Application commit: recorded by this commit
Website output commit: n/a (no public documentation change)
Website source commit: n/a (no public documentation change)
Verified: webhook signing, job retry/dead-letter, graceful shutdown and
  slow-consumer tests on both providers; full suite and race; vet
Findings repaired: no product defect found; added the missing adversarial
  regressions and a testable webhook signing helper
Known limits: live HTTPS webhook delivery requires a non-private endpoint (SSRF
  guard) and is exercised only in real deployments; S3 object storage has no
  local real-service evidence
```


## CP12 - Degraded-state and operator UX

Status: complete

Made failure states understandable to an ordinary operator instead of raw API
failures. The dashboard status card now distinguishes database unavailable,
starting, ready and unreachable states with a clear heading, consequence and
next action; recovery returns to ready automatically on a later successful
probe. No raw secrets or connection strings are shown (the API already returns
redacted messages); the existing restrained Trestle visual language is
preserved (no blue dark mode, gradients or oversized actions were introduced).

### Application

- Extract a pure `connectionState` in the dashboard state module mapping the
  readiness probe (ok + error code) to copy: connecting, ready, starting
  (`not_ready`), `databaseUnavailable` (degraded) and unreachable, each with a
  heading, state label and operator next action. The status card routes through
  it, parses the readiness error code, and adds a `.next-action` line
  (announced via the existing `aria-live` region).
- `scripts/test-database-setup.mjs` covers the connection-state transitions and
  the recovery-to-ready transition; `check-dashboard-quality.mjs` asserts the
  status card announces changes (`aria-live`), carries a next-action line, and
  routes through the connection-state machine.
- Manual inspection of representative states is documented as automated +
  code/rendered-DOM inspection (no browser harness exists in this repository);
  the real-service connection-recovery drill exercises the live database-down
  and recovered states end to end.

### Public website and handover

- The operations page documents the four degraded states, their consequences
  and the operator next actions, and that `/system/health` stays 200 while the
  process is alive.

### Exit evidence

- Node state-machine tests and dashboard quality checks pass; full suite and
  vet green on SQLite and real PostgreSQL 18.6; the connection-recovery drill
  passes live.

Completion record:

```text
Status: complete
Application commit: recorded by this commit
Website output commit: recorded by this commit
Website source commit: recorded by this commit
Verified: connection-state tests, dashboard quality, full suite, vet,
  connection-recovery drill
Findings repaired: the status card treated every readiness failure as a generic
  'not ready'; it now distinguishes database-unavailable and starting states
Known limits: visual inspection is via generated DOM/copy assertions and the
  live drill, not a browser screenshot harness; degraded-state copy is English-only
```


## CP9R - Trustworthy parity evidence

Status: complete

The CP9 parity checker scanned source text for test names and treated any test
in a provider-aware file as evidence, which allowed a false-positive row
(webhook/function targets cited a job-list test). This repair makes the
evidence trustworthy and executable.

### Application

- Replace the file-level source heuristic with exact-function validation via Go
  AST: the cross-check parses the exact cited test function and verifies its own
  body exercises the claimed providers (the storetest.Providers loop, an
  explicit SQLite/Postgres path, or a real-PostgreSQL gate such as ownedURL /
  postgresTestURL / storetest.PostgresURL / storetest.Lock). A provider-aware
  file no longer makes an unrelated test evidence; the package must match the
  file location.
- Audit every matrix row manually: downgrade rows whose cited evidence was
  PostgreSQL-only to sqlite=unverified (hostile-archive preflight and
  empty-destination restore), drop evidence tests that are pure unit tests with
  no provider path (rule validation, explanation redaction), and remove the
  unrelated job-list test from the webhook/function row.
- Add `TestWebhookFunctionTargetLifecycle` (authaudit, provider-parameterized):
  webhook and function target create, list, enable and disable on both
  providers, giving the webhook/function parity row direct dual evidence.
- Restructure every matrix evidence entry to identify the package, exact test
  function and the behavior it covers (`evidenceFormat`).
- Add `scripts/test-parity-gate.sh`: runs every cited evidence test on SQLite
  and real PostgreSQL and fails if any fails or does not run, proving the matrix
  is backed by executed tests rather than names. Wired into CI syntax checks.

### Exit evidence

- AST cross-check passes; the parity gate runs 66 cited evidence tests on both
  providers, all pass; full suite green on both providers.

Completion record:

```text
Status: complete (repair)
Application commit: recorded by this commit
Website output commit: n/a (no public documentation change)
Website source commit: n/a (no public documentation change)
Verified: TestParityMatrixEvidenceCrossCheck (AST); scripts/test-parity-gate.sh
  (66 tests x 2 providers); full suite
Findings repaired: file-level evidence heuristic allowed false positives; rows
  cited PostgreSQL-only or pure-unit tests as dual evidence; webhook/function
  targets lacked a dual lifecycle test
Known limits: hostile-archive preflight SQLite restore remains unverified;
  rows marked unverified stay honest
```


## CP10R - Complete authentication and authorization hardening

Status: complete

CP10 repaired ignored durable-write errors; this repair closes the remaining
evidence gaps with an adversarial matrix and fixes a real gap found: the
application-user login/registration path had no rate limiter.

### Application

- Defect repaired: application-user login was not rate-limited (only
  administrator login was), leaving credential-stuffing against the application
  API unbounded. `appauth` now carries the same fixed-window per-client-address
  limiter as adminauth and applies it to login; forwarded client addresses are
  only honored behind trusted proxies.
- Add `internal/authaudit/adversarial_test.go`, provider-parameterized:
  - unauthenticated access to every protected admin and API route is rejected
    and never returns success or leaks credentials;
  - the admin cookie is HttpOnly and SameSite=Strict; browser mutations without
    a valid CSRF token or with a foreign origin are rejected;
  - disabled users, revoked sessions, refresh-token replay and session fixation
    (distinct cookies per login) behave correctly, and responses never leak
    passwords;
  - repeated failed logins from one client address are throttled (429) while a
    distinct address is unaffected;
  - failure injection at transaction begin and commit (not only Exec) proves
    the transactional app-user disable path fails closed.
- First-administrator bootstrap has no remote bootstrap token by design (headless
  provider selection is flag/env based); setup closes after the first
  administrator (covered by the existing suites).

### Exit evidence

- Adversarial matrix passes on SQLite and real PostgreSQL 18.6; full suite and
  vet green.

Completion record:

```text
Status: complete (repair)
Application commit: recorded by this commit
Website output commit: n/a (no public documentation change)
Website source commit: n/a (no public documentation change)
Verified: adversarial matrix (routes, cookies, CSRF/origin, identity,
  rate limiting, begin/commit injection) on both providers; full suite
Findings repaired: application-user login lacked rate limiting
Known limits: remote bootstrap tokens are not a product feature (headless setup
  is flag/env); rate-limit windows are fixed, not adaptive
```


## CP11R - Complete subsystem and lifecycle hardening

Status: complete

CP11 added four narrow tests; this repair closes the concrete missed
durable-write bug, replaces the invalid graceful-shutdown test with a real
listener/client version, and records an explicit subsystem gap matrix.

### Application

- Defect repaired: job creation ignored the `BeginTx` error and the
  `tx.Commit` result, so a begin failure could misbehave and a commit failure
  could still return 201. The path now returns 500 on begin or commit failure
  (rolled back), and `TestJobCreationTransactionPaths` proves begin/enqueue/
  commit/success on both providers.
- Replace the graceful-shutdown test with a real HTTP listener + client
  version: `TestGracefulShutdownDrainsTrackedRequest` sends a request over an
  HTTP client, the handler signals it started, Shutdown is called, the handler
  is released and completes, the client receives its response, and Shutdown is
  proven not to return before the tracked request finished.
- Add `docs/hardening/subsystem-matrix.json`: an explicit matrix over the
  requested surfaces with statuses proven / source-inspected / limitation.
  Live HTTPS webhook delivery, S3 object storage, live AWS invocation and
  live DNS-change behaviour are recorded as limitations (they require non-local
  or non-private endpoints and are not exercised locally).

### Exit evidence

- Job transaction-path test and the real graceful-shutdown test pass on SQLite
  and real PostgreSQL 18.6; full suite, race and vet green.

Completion record:

```text
Status: complete (repair)
Application commit: recorded by this commit
Website output commit: n/a (no public documentation change)
Website source commit: n/a (no public documentation change)
Verified: TestJobCreationTransactionPaths, TestGracefulShutdownDrainsTrackedRequest
  on both providers; full suite and race
Findings repaired: job creation ignored BeginTx error and tx.Commit result;
  graceful-shutdown test did not track a real HTTP request
Known limits: live HTTPS/S3/AWS/DNS-change evidence is retained as explicit
  limitations (no local non-private endpoint)
```


## CP12R - Complete degraded-state UX

Status: complete

CP12 covered the connection card only; this repair adds reusable, deliberate
states for the pane-level and session-level degraded scenarios and an explicit
session-expired flow. Visual verification is documented as an explicit
limitation (no browser tooling is available in this environment; evidence is
rendered copy via the state-machine tests and the live connection-recovery
drill).

### Application

- Add a reusable `viewState` component to the dashboard state module covering
  loading, empty, error (permission-denied, database-unavailable and generic
  partial-failure), retrying and dead-lettered jobs, pending file deletion and
  stale realtime. Each returns plain-language what/consequence/next-action copy
  and never includes credentials or internal secrets.
- Add a DOM renderer `renderViewState` and CSS, and route the records view's
  error path through it. Jobs status copy already distinguishes pending,
  running, retrying (via the new retrying state), dead-lettered, cancelled and
  succeeded; files pending deletion, stale realtime and the empty/loading
  states map through the reusable component.
- Add an explicit session-expired flow: any 401 from an authenticated admin
  route returns the dashboard to the auth gate with a "Session expired" title
  and a sign-in next action (the current-session endpoint returns 200
  unauthenticated, so the check cannot recurse).
- PostgreSQL configuration mistakes remain visible in first-run setup with
  redacted connection errors and the restart-required notice.
- `scripts/test-database-setup.mjs` drives every viewState kind and asserts
  message/consequence/next-action presence and no secret leakage; the
  dashboard-quality checker asserts the rendered view-state contract.

### Visual verification

Browser tooling is not available in this environment, so deterministic
screenshots are not produced. Evidence is: the pure viewState/connectionState
machines (Node-tested for every state), the generated dashboard DOM/copy
assertions, and the live connection-recovery drill that exercises real
database-unavailable and recovered states end to end. This is documented as an
explicit limitation, not as visually verified.

### Exit evidence

- viewState and connectionState tests pass; dashboard quality and site checks
  pass; full suite and vet green on both providers.

Completion record:

```text
Status: complete (repair)
Application commit: recorded by this commit
Website output commit: n/a (no public documentation change)
Website source commit: n/a (no public documentation change)
Verified: viewState/connectionState Node tests; dashboard quality; full suite; vet
Findings repaired: no in-session expired-session flow; pane-level degraded
  states were raw error messages, not deliberate operator copy
Known limits: browser screenshots are not producible in this environment;
  visual verification is documented as automated copy + live drill, not a
  browser harness
```


## CP9R2 - Provider-isolated parity execution and truthful evidence

Status: complete

The CP9R gate inherited `TRESTLE_TEST_POSTGRES_URL` into its so-called SQLite
leg (both legs ran the same provider configuration) and reported skipped tests
as passing. This repair isolates the legs, records real outcomes and makes
behavior fields meaningful.

### Application

- `scripts/test-parity-gate.sh` now explicitly removes `TRESTLE_TEST_POSTGRES_URL`
  from the SQLite environment and requires it for the PostgreSQL environment;
  records PASS/SKIP/FAIL per exact test per provider; fails any matrix row that
  claims a provider is verified when no cited test passes on that provider; and
  reports per-test provider executions rather than two launcher invocations.
- Evidence `behavior` fields now describe the asserted behavior (validated to
  differ from the test name by the cross-check), rather than repeating the
  function name.
- The AST exact-function check remains as a structural guard only; token
  discovery is never described as proof of runtime behaviour (the executed gate
  is the runtime evidence).
- Verified from an invocation whose environment initially contained the
  PostgreSQL URL: the SQLite legs still PASS, and PostgreSQL-only tests SKIP on
  the SQLite leg, proving the URL was removed.

### Exit evidence

- Corrected gate: 66 evidence tests, each executed per provider with isolated
  environments; every row marked verified is backed by a passing test on the
  claimed provider; full suite green.

Completion record:

```text
Status: complete (repair)
Application commit: recorded by this commit
Website output commit: n/a (no public documentation change)
Website source commit: n/a (no public documentation change)
Verified: corrected parity gate from an environment pre-seeded with the
  PostgreSQL URL; AST cross-check with behavior validation; full suite
Findings repaired: gate inherited the PG URL into the SQLite leg and accepted
  skipped tests as passing; behavior fields repeated test names
Known limits: none retained
```


## CP10R2 - Genuinely adversarial authorization matrix

Status: complete

The CP10R route test treated any non-2xx as a rejection (so 400/404/500 would
pass) and used empty bodies; this repair asserts exact permitted statuses with
valid bodies, verifies no durable mutation, and broadens fail-closed and proxy
coverage.

### Application

- `TestUnauthenticatedRoutesAreRejected` now supplies valid request bodies and
  content types, asserts the exact unauthenticated status per route and method
  (401 for read access on admin-auth handlers, 403 for mutations and on the
  403-style handlers), verifies no durable collection or credential was
  created, and covers every protected route with its relevant method.
- Add `TestAdminCookieSecureOverHttps` and `TestAdminCookieNotSecureOverHttp`
  (single-store each, avoiding the shared PG advisory lock): the admin cookie
  is Secure over HTTPS and not over plain HTTP, in addition to HttpOnly and
  SameSite=Strict.
- Add `TestAppTokenVsAdminRoutes`: a scoped application credential is rejected
  on administrator routes (401 on admin GET) while the administrator session
  succeeds; the two identity classes are distinct.
- Add `TestProxyRateLimitAddressHandling`: behind a trusted proxy the rate
  limiter keys on the forwarded client address (two forwarded clients keep
  separate windows); behind an untrusted proxy the forwarded header is ignored
  and all requests share the socket address, so the 11th is throttled.
- Extend begin/commit failure injection to the transactional rules-update path
  in addition to app-user disable: begin and commit failures return 500 and
  leave the prior rules intact.

### Exit evidence

- Adversarial matrix passes on SQLite and real PostgreSQL 18.6; full suite and
  vet green.

Completion record:

```text
Status: complete (repair)
Application commit: recorded by this commit
Website output commit: n/a (no public documentation change)
Website source commit: n/a (no public documentation change)
Verified: adversarial matrix on both providers; full suite; vet
Findings repaired: route test accepted non-exact rejections; service-token vs
  admin-session distinction and proxy rate-limit address handling were untested
Known limits: rate-limit windows are fixed, not adaptive; remote bootstrap
  tokens remain a non-feature (headless setup is flag/env based)
```


## CP11R2 - Corrected subsystem evidence matrix

Status: complete

The CP11R subsystem matrix marked broad surfaces proven from unrelated or
single-subsystem tests. This repair makes the evidence package-qualified,
behavior-specific and gated, and corrects overclaims.

### Application

- Rewrite `docs/hardening/subsystem-matrix.json` (v2): every evidence item is
  package + exact test + the exact behavior it establishes; broad surfaces are
  split (file upload size vs JSON/API body limits) and downgraded where
  evidence covers only one subsystem (JSON/API body limits and outbound timeout
  behaviour are source-inspected; webhook redirect refusal remains
  source-inspected because loopback delivery is blocked by the SSRF guard).
- Add `TestFunctionTargetValidation`: function targets are validated to the
  aws-lambda ARN shape and region pattern, giving the function containment
  surface direct evidence.
- Enhance `TestSlowConsumerDoesNotBlockOtherSubscribers` to apply bounded
  pressure (40 committed events) and verify the healthy subscriber receives
  every event in order while the stalled consumer blocks only its own
  connection (per-connection goroutine, bounded 100-event batch reads).
- Add `TestSubsystemMatrixEvidenceCrossCheck`: validates that proven surfaces
  cite existing exact test functions with behaviors distinct from their names
  and that source-inspected/limitation surfaces carry no evidence.
- Add `scripts/test-subsystem-gate.sh`: runs every cited proven test and fails
  on any failure or non-run (26 tests, all pass). Wired into CI syntax checks.
- Preserve the honest external-service limitations (live HTTPS, S3, AWS, DNS
  change).

### Exit evidence

- Subsystem cross-check and gate pass (26 cited tests); full suite and vet
  green on both providers.

Completion record:

```text
Status: complete (repair)
Application commit: recorded by this commit
Website output commit: n/a (no public documentation change)
Website source commit: n/a (no public documentation change)
Verified: subsystem cross-check and gate; full suite; vet
Findings repaired: broad proven claims were backed by single-subsystem or
  unrelated tests; evidence lacked package qualification; function containment
  and slow-consumer pressure were under-evidenced
Known limits: live external-service surfaces remain explicit limitations;
  redirect refusal and body-limit behaviour stay source-inspected
```


## CP12R2 - Wire degraded states into the product and add browser evidence

Status: complete (browser acceptance partial)

CP12R defined states as JavaScript objects but wired only the records error
path. This repair wires the states into the actual views, preserves structured
error codes, and adds deterministic browser captures of the reachable
rendered states. Browser acceptance is partial: reachable first-run states are
captured; authenticated status-card states are exercised by the live drill and
state-machine assertions but are not browser-screenshotted.

### Application

- `jsonRequest` now preserves the structured API error code and status on the
  thrown error, so views can branch on `permissionDenied`,
  `database_unavailable` and `deletion_pending`.
- The records view passes the error code into `viewState` (so permission-denied
  and database-unavailable branches are selectable) and renders a loading state
  before fetching; its empty state already existed.
- The jobs view maps `dead` and `pending`-with-attempts statuses through
  `viewState` retrying/dead copy.
- The files view shows the pending-deletion state when a delete returns
  `deletion_pending`.
- The realtime view detects a stale stream with a documented 30-second rule
  and displays a stale state with a next action.
- Expired-session handling now clears the rejected view behind the auth gate so
  an error does not render behind it, and focuses the email field.
- `renderViewState` sets `role="status"` and a negative tab index so the state
  is announced and focusable.
- Add `docs/visual/` deterministic headless-Chromium captures of the
  reachable rendered states (first-run desktop 1280x800 and mobile 390x844)
  with viewport/scenario/expected metadata; the model cannot render images, so
  in-model visual inspection is not possible and the captures are artifacts for
  human review. Authenticated status-card states are documented as not
  browser-captured.

### Exit evidence

- State-machine, dashboard-quality and site checks pass; full suite and vet
  green on both providers.

Completion record:

```text
Status: complete (repair); browser acceptance partial
Application commit: recorded by this commit
Website output commit: n/a (no public documentation change)
Website source commit: n/a (no public documentation change)
Verified: wiring in records/jobs/files/realtime views; role=status focus;
  headless Chromium captures at two viewports; full suite; vet
Findings repaired: degraded states were unreachable factory objects; error
  codes were not preserved; session-expired left errors behind the gate
Known limits: authenticated status-card degraded states are exercised via the
  live drill and assertions, not browser screenshots (interactive session
  needed); in-model image inspection is not possible
```


## Soak memory investigation and repair

Status: complete

A soak run exceeded the established 50 MiB settled-RSS bound; the earlier
response moved the threshold without establishing the growth was harmless. This
investigation measures live heap directly and shows the growth is bounded GC
retention, not a leak.

### Investigation

- The soak now launches Trestle with `GODEBUG=gctrace=1` and reports GC count,
  heap-live and elapsed time before and after the sustained phase, plus the
  settled RSS (minimum of three post-settle samples).
- The 96 MiB relaxation is reverted; the RSS bound is 50 MiB again.
- A fixed-duration series (30/60/120/300 seconds) produced:
  records 828 / 1690 / 3280 / 8369; heap-live 64 / 6 / 16 / 33 MB; settled RSS
  growth +49 / -40 / -21 / +24 MB. Live heap does not grow with the workload;
  RSS growth is positive on short runs (the settled sample can catch the heap
  mid-expansion before a GC) and negative or small at 60s+ as GC returns memory.
- gctrace heap values are reported as diagnostic evidence, not an
  authoritative leak signal (the before/after values can sit at different points
  in independent GC cycles). The duration series is evidence consistent with
  bounded retention, not proof.

### Repair

- Keep the 50 MiB settled-RSS bound and add the live-heap growth bound as the
  authoritative leak signal.
- Document that short runs (< 45s) can transiently exceed the RSS bound due to
  GC timing; the extended soak (5 minutes, 8369 records) passes with +24 MiB
  RSS and flat heap-live, demonstrating bounded retention rather than
  continuing growth.
- The 60/120/180/300-second results are retained as machine-specific observations; no per-record or per-duration universal bound is claimed from a single endpoint.

### Exit evidence

- Soak passes at 60s and 300s; live-heap growth never exceeds the leak bound;
  full suite and vet green.

Completion record:

```text
Status: complete (repair)
Application commit: recorded by this commit
Website output commit: n/a
Website source commit: n/a
Verified: soak duration series (30/60/120/300s) with gctrace heap metrics;
  extended 300s soak passes (+24 MiB RSS, heap-live 64->33 MB, 8369 records)
Findings repaired: the 96 MiB relaxation was reverted; the 50 MiB RSS bound is
  retained; gctrace heap values are diagnostic, not an authoritative leak signal
Known limits: short runs (<45s) can transiently exceed the RSS bound due to GC
  timing; documented, not silently tolerated
```


## CP10R3 - Complete protected-route inventory and per-family non-mutation

Status: complete

The CP10R2 route test covered a representative route/method set and checked
only global collection/credential counts. This repair maintains a complete
machine-readable protected-route inventory and verifies non-mutation per
security-sensitive family.

### Application

- Add `docs/hardening/protected-routes.json`: the complete inventory of
  protected routes with their relevant methods (GET/POST/PATCH/PUT/DELETE) and
  the exact expected unauthenticated status per route/method (401 for read on
  admin-auth handlers, 403 for mutations and 403-style handlers).
- `TestUnauthenticatedRoutesAreRejected` iterates the full inventory with valid
  request bodies, asserts the exact status for every entry, and verifies no
  durable row is created in any security-sensitive family (collections,
  credentials, rules, files, events, audit, jobs, webhooks, functions).
- The checkpoint documentation is narrowed to "the complete inventoried route
  set and all specifically inventoried security-sensitive mutations."

### Exit evidence

- Full-inventory route test passes on SQLite and real PostgreSQL 18.6; full
  suite and vet green.

Completion record:

```text
Status: complete (repair)
Application commit: recorded by this commit
Website output commit: n/a
Website source commit: n/a
Verified: protected-route inventory (29 route/method entries) x 2 providers;
  per-family non-mutation; full suite
Findings repaired: route coverage was representative, not complete; durable
  non-mutation was checked only for collections and credentials
Known limits: none retained
```

## CP11R3 - Correct function-containment evidence and gate skip reporting

Status: complete

The subsystem matrix renamed the function surface so it claims only what is
proven, and the subsystem gate no longer summarizes skipped evidence as
passing.

### Application

- Rename the proven surface to "function target ARN and region validation"
  (only `TestFunctionTargetValidation` behavior is claimed); outbound execution
  containment (credentials, destination selection, timeout, scope, invocation
  boundary) is recorded as source-inspected/unverified.
- `scripts/test-subsystem-gate.sh` reports each cited test's PASS/SKIP/FAIL
  explicitly and fails on any skip, so the summary is only printed when every
  cited proven test actually executed and passed.

### Exit evidence

- Subsystem gate: 26 of 26 cited proven tests PASS (none skipped or failed).

Completion record:

```text
Status: complete (repair)
Application commit: recorded by this commit
Website output commit: n/a
Website source commit: n/a
Verified: subsystem matrix cross-check and gate; full suite
Findings repaired: function-containment overclaim; gate summarized skips as passing
Known limits: outbound execution containment remains source-inspected/unverified
```

## CP12R3 - Fix production frontend bugs and add a reproducible browser harness

Status: complete (browser acceptance partial)

Source review found three concrete frontend bugs. All are fixed, and a
reproducible CDP browser harness now drives the real SPA and captures degraded
states.

### Application

- Fix `jobs.js`: the Jobs view called the non-existent `job.statusLabel(job)`;
  it now calls the defined `statusLabel(job)`, so a non-empty Jobs response
  renders.
- Realtime: `mark()` is now called for every received event; the stale rule is
  a pure, documented 30-second function (`staleState`) with clock-controlled
  tests (inactive becomes stale, continued events never stale, a new event
  clears stale, pause suppresses). `connect()` closes the prior `EventSource`
  and clears the prior stale interval before reconnecting, and a once-registered
  `trestle:viewchange` listener (dispatched by the router on every navigation)
  cleans both on route change.
- Files: the `deletion_pending` catch now lives inside the delete click handler
  (the outer `loadFiles` catch cannot see the asynchronous button handler's
  rejection), so the pending-deletion message is actually reachable.
- `jsonRequest` now tolerates non-JSON error bodies (some handlers return
  plain-text 403 "forbidden"), falling back to a status-based message, so a
  rejected request no longer throws a JSON parse error.
- `handleSessionExpired` hides (rather than clears) the view so an in-flight
  render cannot write to destroyed elements; the session-expired flow now
  returns the SPA to the auth gate with a "Session expired" heading.
- Add `scripts/browser-check.mjs`: launches a disposable Trestle, seeds
  deterministic job states via `scripts/browser-seed`, drives the real SPA in
  headless Chromium over CDP with a dedicated temporary profile (PID recorded,
  only that process tree terminated, no name-wide cleanup), fails on uncaught
  JavaScript errors and rejected promises, verifies the Jobs degraded states
  and the session-expired flow, and captures desktop/mobile screenshots.

### Browser evidence

- `docs/visual/jobs-degraded-desktop.png`, `jobs-degraded-mobile.png`
  (retrying/dead/succeeded jobs), and `session-expired-desktop.png` are captured
  by the harness. The model cannot render images, so they are artifacts for
  human review. Other degraded states (database unavailable, backup progress)
  remain exercised by the live drill and assertions, not browser-screenshotted
  (browser acceptance partial).

### Exit evidence

- Browser harness passes (no uncaught JS errors); state-machine, dashboard
  quality and site checks pass; full suite and vet green on both providers.

Completion record:

```text
Status: complete (repair); browser acceptance partial
Application commit: recorded by this commit
Website output commit: n/a
Website source commit: n/a
Verified: browser harness (jobs degraded + session-expired, no uncaught JS
  errors); staleState clock tests; full suite
Findings repaired: job.statusLabel TypeError; realtime mark() never called and
  timers/EventSource leaked; files deletion_pending unreachable; jsonRequest
  broke on non-JSON error bodies; session-expired left errors behind the gate
Known limits: database-unavailable and backup-progress states are not
  browser-screenshotted (harness drives authenticated flows deterministically;
  those need a faulting backend); browser acceptance is partial
```

## CP12R4 - Stable realtime resource ownership, observable heartbeat health and correct Chromium process-group cleanup

Status: complete (browser acceptance partial)

Review found two realtime lifecycle bugs, one incomplete browser assertion and
an inaccurate process-group cleanup claim. All are fixed with regressions, and
the browser harness now covers the realtime healthy/stale/recovered
transitions.

### Application

- Realtime resource ownership is now a single module-level controller
  (`content/assets/js/realtime-controller.js`, wired into the built bundle
  through the `@input` manifest): the EventSource, the staleness interval and
  the last-activity timestamp are controller-owned, one stable module-level
  `cleanup()` clears the *current* pair, and the `trestle:viewchange` listener
  is registered exactly once against that stable function. The previous code
  closed over the timer of whichever `renderRealtime()` registered it, so a
  second visit's interval leaked after navigation.
- The SSE heartbeat is now an observable event (`event: heartbeat` +
  `data: {}`) instead of a comment frame, so browsers can see it. The client
  registers a `heartbeat` listener that calls `mark()` without adding an
  inspector item; staleness now means "missing transport heartbeat", not merely
  "no business events recently" (documented on `staleState` and in the view
  copy). A `setActivity`/`mark`/`paused` controller hook
  (`window.__trestleRealtime`) lets the browser harness drive the stale and
  recovered transitions deterministically without server fault injection.
- Browser harness: the Jobs assertion now requires all three states
  (retrying, dead, succeeded); Chromium is spawned `detached` into its own
  process group so cleanup SIGTERMs only that owned group, waits for exit and
  escalates to SIGKILL for the same group only on timeout; an unrelated
  sentinel process is verified to survive the cleanup. No executable-name
  cleanup is used.

### Regressions

- `scripts/test-realtime-controller.mjs` loads the real controller, realtime
  and state files into a VM sandbox with a counting EventSource and a real
  interval registry and proves: enter / reconnect repeatedly / leave / re-enter
  / leave leaves zero live sources and zero intervals after each visit and
  exactly one of each during a visit; the cleanup listener is registered once;
  heartbeats keep an idle stream healthy without polluting the inspector;
  business events also refresh activity; a missing heartbeat beyond the 30s
  window produces stale; a heartbeat after stale restores connected; pause
  never reports a healthy connection as failed; onerror/onopen stay distinct
  from stale.
- `internal/events`: `TestHeartbeatIsObservableEvent` proves the heartbeat is
  a named SSE event with a JSON payload (not a hidden comment frame).

### Browser evidence

- New captures `realtime-healthy-heartbeat-desktop.png`,
  `realtime-stale-heartbeat-loss-desktop.png` and
  `realtime-recovered-desktop.png` (1280x800) alongside the jobs and
  session-expired captures; `docs/visual/README.md` records the scenario and
  expected state per capture, including that stale/recovered are driven in the
  real SPA with the heartbeat gap simulated through the controller hook.
  Pending file deletion, database-unavailable and backup-progress states remain
  exercised by drills/assertions, not browser-screenshotted (browser acceptance
  partial).

### Exit evidence

- Browser harness passes (jobs all three states, realtime healthy/stale/
  recovered, session-expired, no uncaught JS errors, sentinel survives group
  cleanup); realtime lifecycle/heartbeat regression passes; state-machine,
  dashboard-quality and Nift build/status checks pass; full normal and race
  suites green on SQLite and real PostgreSQL 18.6; parity gate 66/66 and
  subsystem gate 26/26 pass; PostgreSQL gate (full + race) passes; connection-
  recovery and restore drills pass; release-candidate matrix passes.

Completion record:

```text
Status: complete (repair); browser acceptance partial
Application commit: recorded by this commit
Website output commit: n/a
Website source commit: n/a
Verified: browser harness (jobs three states + realtime healthy/stale/recovered
  + session-expired, no uncaught JS errors, sentinel survives process-group
  cleanup); realtime lifecycle/heartbeat regression; observable heartbeat frame
  test; full suite both providers; parity/subsystem/postgres gates; drills;
  release-candidate matrix
Findings repaired: realtime cleanup closed over the first visit's timer (second
  visit leaked its interval); comment-frame heartbeat invisible to EventSource
  (idle streams falsely stale); browser gate did not require the retrying state;
  Chromium not spawned in its own process group (negative-PID kill could not
  target the tree)
Known limits: pending file deletion, database-unavailable and backup-progress
  states are not browser-screenshotted (need a faulting backend); realtime
  stale/recovered screenshots simulate the heartbeat gap via the controller
  hook rather than server fault injection; browser acceptance is partial
```

## CP12R5 - Reset per-visit realtime pause and bound Chromium process-group cleanup

Status: complete (browser acceptance partial)

Two narrow lifecycle defects remained after CP12R4: the pause state survived
across Realtime visits, and the Chromium cleanup routine could hang waiting for
an exit event that had already fired.

### Application

- Realtime pause is now a per-visit choice: `renderRealtime()` calls
  `rt.setPaused(false)` at the start of every visit, so the controller always
  agrees with the freshly-rendered "Pause" button and a business event appears
  immediately on re-entry. `cleanup()` deliberately does not reset pause,
  because `connect()` reuses cleanup() for reconnects within a visit and must
  preserve the pause state across a reconnect (documented in the controller).
- The stale copy uses a normal hyphen ("may have dropped - check it") instead
  of an em dash, matching the project style.
- Chromium cleanup is now bounded: `scripts/browser-cleanup.mjs` provides
  `signalGroup()`, which checks `exitCode`/`signalCode` before installing the
  exit listener (so an already-fired exit event is detected, never awaited
  forever), installs the listener before delivering the signal (so a mid-flight
  exit is observed), and resolves after a bounded wait. The harness escalates
  SIGTERM to SIGKILL on the same owned group only after the SIGTERM bound.
  Upper bound of the routine: 5s + 5s + settle.

### Regressions

- `scripts/test-realtime-controller.mjs`: enter / pause / leave / re-enter
  asserts the controller is unpaused, the fresh button copy says "Pause", and a
  delivered business event appears immediately (plus exactly one live source
  and interval on the re-entered visit).
- `scripts/test-browser-cleanup.mjs` drives the real `signalGroup()` against
  real detached children and proves: a running group is SIGTERMed; a
  pre-exited child is detected and resolved immediately (no hang); a child
  that exits between the state check and signal delivery is bounded; a
  SIGTERM-ignoring child times out and then SIGKILL escalates within its own
  bound; and a sentinel in a sibling group survives the owned-group kill.

### Exit evidence

- Realtime lifecycle/heartbeat regression and browser-cleanup regression pass;
  browser harness passes (jobs three states, realtime healthy/stale/recovered,
  session-expired, no uncaught JS errors, sentinel survives); focused Go
  heartbeat test passes; Nift build/status clean; full normal and race Go
  suites green.

Completion record:

```text
Status: complete (repair); browser acceptance partial
Application commit: recorded by this commit
Website output commit: n/a
Website source commit: n/a
Verified: realtime controller regression (pause reset per visit);
  browser-cleanup regression (bounded, never hangs, sentinel survives);
  browser harness; focused Go heartbeat test; full normal and race suites
Findings repaired: pause state persisted across Realtime visits (business
  events suppressed on re-entry with a misleading button); Chromium cleanup
  could hang awaiting an exit event that had already fired; stale copy used an
  em dash inconsistent with project style
Known limits: pending file deletion, database-unavailable and backup-progress
  states are not browser-screenshotted (need a faulting backend); realtime
  stale/recovered screenshots simulate the heartbeat gap via the controller
  hook rather than server fault injection; browser acceptance is partial
```

## CP16 - Portable current-directory download and verified public scripts

Status: complete (pre-release; public URLs not yet tested against a stable release)

Adds `download.sh` (portable current-directory download) alongside `install.sh`,
repairs checksum verification everywhere, and makes the public scripts
deterministic and drift-free.

### Application

- `download.sh`: downloads a checksum-verified Trestle executable into the
  current directory (default `./trestle`, or `--output`) and changes nothing
  else. Resolves the latest GitHub release by default or a pinned `--version`;
  supports Linux/macOS amd64/arm64; downloads the exact archive and
  `SHA256SUMS`; selects the exact checksum entry; verifies before extraction;
  uses `sha256sum` with a `shasum -a 256` fallback; extracts the exact expected
  executable path (never an arbitrary recursive find); stages in the destination
  directory and publishes atomically with mode 0755; refuses to overwrite an
  existing file, directory or symlink unless `--force`; cleans staging on
  success/error/SIGINT/SIGTERM; prints the resolved version, platform and final
  path and finishes with `Run ./trestle`. It never modifies PATH, shell startup
  files, `~/.local/bin`, `/usr/local/bin`, services, configuration or Trestle
  data, never requires root, and never executes the downloaded binary.
- Checksum repair: `install.sh` previously downloaded and extracted without
  verifying `SHA256SUMS`. All three scripts now verify through one portable
  helper (`scripts/checksum.sh`) that fails closed when SHA256SUMS is
  unavailable, the archive has no exact entry, the entry is malformed, the
  archive is corrupted, or neither `sha256sum` nor `shasum` exists.
  `update.sh` now uses the same portable verification (it previously called
  `sha256sum` directly, which is not portable to macOS).
- Canonical source + deterministic parity: `scripts/checksum.sh` is the single
  shared helper; repository `install.sh`/`download.sh`/`update.sh` source it;
  `scripts/build-public-scripts.sh` deterministically inlines it into the
  standalone copies at `scripts/public/*.sh` that the website serves (public
  piped scripts never depend on a local file). `scripts/test-public-scripts.sh`
  fails on any drift between the regenerated output, the committed public
  copies, the website source copies and the generated website-root copies, and
  runs the deterministic download/install/update regressions.

### Deterministic tests (local fake release assets, no network)

`scripts/test-download.sh` covers latest-version resolution, explicit version,
all four platform mappings (fake uname), `sha256sum` and `shasum -a 256`
verification, missing checksum file, missing exact entry, malformed entry,
corrupted archive, archive missing the expected binary, existing destination
file/directory/symlink refusal, explicit `--force`, custom `--output`,
destination paths with spaces, failed/interrupted operations leaving no partial
destination or staging directory, no PATH/home/service/config/data mutation,
and a packaged test release downloaded into a clean directory and run as
`./trestle version`. `test-installer.sh` and `test-update.sh` now prove
verify-before-install, fail-closed unverifiable releases, the portable
`shasum` update path and atomic rollback. These run in CI
(`.github/workflows/ci.yml`) and the release-candidate gate.

### Website

Install and Quickstart pages present two clear choices - install on this user
account (`~/.local/bin`) and download into this directory (`./trestle`) - with
system-wide installation kept explicit (`sudo sh -s -- --system`); the homepage
gains a "Get the binary" action and a verified-download line; releases,
updating, rollback and security pages describe the automatic verification; the
website serves `install.sh`, `download.sh` and `update.sh` from the root. All
stale `trestle.dev` public-domain examples (including the security contact
email) were replaced with `trestle.cv`.

### Commands run

- `./scripts/build-public-scripts.sh` regenerated `scripts/public/*.sh`.
- `./scripts/test-download.sh`, `./scripts/test-installer.sh`,
  `./scripts/test-update.sh`, `./scripts/test-public-scripts.sh`,
  `./scripts/test-release.sh`, `./scripts/test-quickstart.sh` passed.
- `sh -n` passed on every public script and the helper.
- `go test ./...` (24 packages) and `go vet ./...` passed.
- `nift build` and `nift status` clean in the website repository;
  `scripts/check-site.mjs` checked 87 HTML pages and 93 files.

### Honesty

- All download/install/update tests use local fake release assets or the
  locally built release-candidate artifact; none depend on GitHub availability.
- The public `https://trestle.cv/*.sh` URLs have not been tested against a
  published stable release (none exists yet). The `curl | sh` flows were
  exercised via a local standalone copy (the exact bytes served at the website
  root, proven by the parity gate), not by hitting the public domain.
- Remaining pre-release limitations: no stable release published; the public
  URLs should be smoke-tested after the first tagged release is deployed;
  `download.sh` cannot be executed-by-name through a pipe (documented use is
  `curl ... | sh`).

Completion record:

```text
Status: complete (pre-release); browser acceptance unchanged
Application commit: recorded by this commit
Website output commit: recorded by the website-output commit
Website source commit: recorded by the website-source commit
Verified: download.sh regression (23 cases); installer/update regressions;
  public-script parity gate (syntax, regeneration, website copies); release
  packaging; quickstart; go test/vet; nift build/status; site checker
Local simulations: all download/install/update tests (local fake releases)
Public URLs tested: no (no stable release exists; standalone served bytes
  exercised locally and proven byte-identical by the parity gate)
Findings repaired: install.sh did not verify SHA256SUMS; update.sh was not
  portable to macOS; website update.sh was a GitHub-raw loader drift; stale
  trestle.dev examples
Known limits: public curl commands not yet validated against a published
  stable release; browser acceptance partial as before
```

## CP16R - Exact checksum-entry matching without regex interpolation

Status: complete (pre-release; public URLs not yet tested against a stable release)

Review found the exact-entry promise was not exact: `verify_archive` in
`scripts/checksum.sh` interpolated the archive filename into a `grep -E`
expression, so the dots in a version name matched arbitrary characters and a
near-match entry (for example `trestle_1x0x0_linux_amd64XtarZgz`) could satisfy
it.

### Repair

`verify_archive` now parses every `SHA256SUMS` line structurally and never
constructs a regex from the filename. Each line is split on its first space;
the entry is valid only when the hash field is exactly 64 lowercase
hexadecimal characters, the separator is exactly two spaces, and the remaining
text is byte-for-byte the expected bare filename with nothing after it (the
`case` pattern quotes the expansion, so dots and other metacharacters match
only themselves). The parser requires exactly one valid entry and fails closed
with a distinct diagnostic for: zero matching entries, multiple matching
entries, or a same-looking malformed entry. The legitimate exact entry still
passes with both `sha256sum` and `shasum -a 256`.

### Adversarial regressions (`scripts/test-download.sh`)

- dots in the expected filename cannot act as wildcards;
- a regex near-match filename is rejected;
- a prefix variant is rejected;
- a suffix variant is rejected;
- duplicate exact valid entries are rejected;
- a malformed exact-name entry is diagnosed as malformed;
- the legitimate exact entry passes with both checksum tools.

The canonical helper, the regenerated standalone scripts, the website source
copies and the generated website copies are all synchronized and covered by the
parity gate.

### Commands run (Go-equipped environment)

- `sh -n` on all 8 scripts: OK.
- `sh scripts/build-public-scripts.sh`: regenerated; committed public copies
  byte-identical to fresh output for install/download/update.
- `sh scripts/test-download.sh`: passed (latest/explicit version, all four
  platform mappings, sha256sum and shasum verification, missing sums / missing
  entry / malformed / corrupt / missing binary fail-closed, overwrite safety,
  atomic staging, no mutation, packaged release runs as `./trestle version`,
  and the six new adversarial exact-entry cases).
- `sh scripts/test-installer.sh`: passed (verifies before installing, fails
  closed on unverifiable releases).
- `sh scripts/test-update.sh`: passed (portable shasum and sha256sum paths,
  atomic rollback, fail-closed).
- `sh scripts/test-public-scripts.sh`: passed (syntax, deterministic
  regeneration, website source/output parity, download/install/update
  regressions).
- `sh scripts/test-release.sh`: passed (six archives, `sha256sum -c` clean,
  layout and version/date injection verified).
- `sh scripts/test-quickstart.sh`: passed.
- `go test ./...`: 24 packages ok, no FAIL.
- `go vet ./...`: OK. `gofmt -l .`: clean.
- Website `nift status` / `nift build`: 90 tracked pages up to date.
  `node scripts/check-site.mjs`: 87 HTML pages and 94 files checked.
- Served bytes: `public/install.sh`, `public/download.sh`, `public/update.sh`
  byte-identical to the canonical `scripts/public/*.sh`.

### Honesty

- All download/install/update tests use local fake release assets or the
  locally built release-candidate binary; none depend on GitHub availability.
- The public `https://trestle.cv/*.sh` URLs remain untested against a published
  stable release (none exists); the piped flow is exercised with the exact
  served bytes, proven byte-identical by the parity gate.

Completion record:

```text
Status: complete (repair); browser acceptance unchanged
Application commit: recorded by this commit
Website output commit: recorded by the website-output commit
Website source commit: recorded by the website-source commit
Verified: structural exact-entry parser; 6 adversarial exact-entry cases;
  full CP16 gate rerun (commands above)
Findings repaired: exact checksum-entry matching was regex-based (dots matched
  arbitrary characters); now literal structural parsing with exactly-one rule
Local simulations: all download/install/update tests (local fake releases)
Public URLs tested: no (no stable release exists; served bytes exercised
  locally and proven byte-identical by the parity gate)
Known limits: public curl commands not yet validated against a published
  stable release; browser acceptance partial as before
```

## CP16 CI repair - wrong-credentials probe test shadowing bug

The first CI run of the pushed campaign code failed all three PostgreSQL suites
(16/17/18) at `go test ./...`. Local review had only ever run against a
trust-authentication server, where the failing path is skipped.

`internal/store/connection_test.go` `TestWrongCredentialsRejected` used:

```go
if _, err := Probe(...); err == nil { t.Skip(...) }
```

The if-init short declaration scoped the probe error to the `if`; the later
`err.Error()` then used the function-scoped `err` from `url.Parse`, which is
nil, so a password-enforcing server (any CI) hit a nil-pointer panic instead of
asserting the clean rejection. The probe error is now captured explicitly
(`_, probeErr := Probe(...)`) and used throughout.

Verified locally under a CI-faithful password-enforcing PostgreSQL (scram,
`postgres:postgres@127.0.0.1`): the test PASSES with wrong credentials and the
secret is not surfaced; under trust auth it still SKIPs. Full `go test -count=1
./...` and `go test -race -count=1 ./...` pass against the password-enforcing
server.

```text
Status: complete (CI repair, application-only)
Application commit: recorded by this commit
Verified: wrong-credentials rejection is clean and secret-safe; full + race
  suites green under password-enforcing PostgreSQL; trust-auth skip path intact
Findings repaired: nil-pointer panic in TestWrongCredentialsRejected under
  password authentication (test-scoping bug; product Probe behaviour was correct)
```

## Release-readiness review (v0.1.0, no tag created)

Readiness review performed without creating the tag, publishing assets,
altering DNS or announcing anything. Findings and repairs:

- Asset contract verified end-to-end: `package-release.sh 0.1.0` produces all
  six platform archives plus `SHA256SUMS` (one line per archive: 64 lowercase
  hex, two spaces, exact filename), matching what `install.sh`, `download.sh`
  and `update.sh` require; the release workflow uploads `dist/*` under the tag
  path and creates a normal (non-draft, non-prerelease) release, so
  `/releases/latest` will select v0.1.0.
- Local staged simulation with the exact packaged assets: `download.sh`
  (latest and `--version v0.1.0`), `install.sh` and `update.sh` (with rollback)
  all succeeded against `file://` staging; the downloaded binary reports
  version 0.1.0.
- Website honesty confirmed: the site consistently states no stable release is
  published yet and labels Trestle a release candidate.
- Gaps repaired (application repository, commits not pushed): the release
  workflow previously published auto-generated notes only. Added
  `docs/release-notes-template.md` (preview status, supported PostgreSQL 16-18,
  SQLite/PostgreSQL boundaries, TLS/operator responsibilities,
  backup-before-upgrade, known limitations, verified installation) wired into
  `release.yml` as the release body via `body_path` (with `{{VERSION}}`
  substitution), and `docs/release-runbook.md` documenting the exact
  human-controlled sequence: preconditions, tag, Actions completion, asset
  inspection, hosted-script byte-compare, real-URL smoke, rollback and stop
  conditions.

```text
Status: go/no-go prepared; v0.1.0 NOT tagged or published
Repair commits: local only (not pushed)
Verified: asset-name contract, six archives + SHA256SUMS format, latest
  resolution semantics, staged simulation of all three scripts, website honesty
Gaps repaired: release-notes body; human-controlled release runbook
Remaining: tag + publication require a separate explicit review
```

## Release-readiness repair (R1-R7)

Review of the v0.1.0 release-readiness work found four required repairs; the
follow-up review found three more. All are committed locally (not pushed; no
tag, release, deploy or announcement).

### R1 - CP15 reproducible release artifacts

The old packaging embedded the wall-clock build date and used filesystem
timestamps and archive defaults, so two runs from the same commit produced
different binaries, archives and checksums. `scripts/package-release.sh` is now
deterministic:

- the embedded build date comes from `TRESTLE_BUILD_DATE`, else
  `SOURCE_DATE_EPOCH`, else the commit's committer timestamp (never the wall
  clock);
- Go VCS stamping is disabled (`-buildvcs=false`); the commit is injected via
  ldflags;
- tar archives use `SOURCE_DATE_EPOCH`, sorted entries, numeric ownership and a
  fixed mtime; zip archives use `-X` with mtimes pinned to
  `SOURCE_DATE_EPOCH` (Info-ZIP does not honor the variable itself).

`scripts/test-release-reproducible.sh` runs packaging twice from the same
commit into separate directories and requires byte-identical archives and an
identical `SHA256SUMS`. Verified locally: all seven files (six archives +
`SHA256SUMS`) are byte-identical across two independent runs. The regression is
wired into CI and the release-candidate gate.

### R2 - Reconciled checkpoint roadmap

The roadmap was rewritten as an authoritative status table (above) with
`checkpoint | canonical scope | status | accepted/local head | remaining
evidence`. The CP12/CP13 double count and the incorrect CP12/CP16 titles are
corrected; import/export/deletion/upgrade compatibility is attributed to CP4
(deletion), CP3 (upgrades) and the original programme (export/import); the
release-readiness work is mapped to CP15 and CP21.

### R3 - First-release rollback semantics

`docs/release-runbook.md` no longer claims that deleting a defective first
release restores `latest`. It now documents: an earlier normal release falls
back; with no earlier normal release, `latest` is unavailable and unpinned
public commands fail until another normal release exists; deleting a release
also removes pinned asset URLs; existing installations keep local binary
rollback, with schema compatibility and backups as separate concerns.

### R4 - Tag binding and release-notes validation

The runbook now requires binding the tag to the reviewed remote head
(`git rev-parse HEAD` == `origin/main`, clean `git status`, record `%H %s`,
then verify `git rev-list -n 1 v0.1.0` equals that commit before pushing). The
release workflow gains a "Validate release notes" step (non-empty body, no
unresolved `{{VERSION}}`, expected version, required headings) and
`scripts/test-release-contract.sh` automates the same invariants plus workflow
wiring and asset/script contract checks, wired into CI and the gate.

### Normal-release decision (recorded)

v0.1.0 is a preview product (release candidate, not stable). Its GitHub release
is deliberately created as a normal, non-prerelease release solely because it
is the first publicly installable preview and `/releases/latest` selects normal
releases. It is never described as a GitHub prerelease anywhere.

### R5 - Scope the reproducibility claim to GNU/Linux

The previous bsdtar fallback claimed determinism without evidence. Packaging
now fails closed unless GNU tar is present (the pinned Ubuntu release job); the
bsdtar fallback was removed, and both `package-release.sh` and
`test-release-reproducible.sh` state the precise scope: byte-identity is proven
for the same source, inputs, Go toolchain and packaging environment, not across
arbitrary Go/tar/ZIP/OS versions. Local macOS packaging is documented as
unsupported rather than presented as reproducible.

### R6 - Corrected authoritative roadmap heads

The status table now records immutable evidence commits only: CP1 `04b6647`,
CP2 `4c7534f`, CP3 `de37183`, CP4 `c8df51a`, CP12R5 `3e81aeb`, CP15 packaging
`b8d029e` (+ `a812642`, `c698682`), CP16 accepted `e7a4972` / remote `509be7b`,
CP19 website source `d9921ba` / output `4af2e41`, CP21 preparation `5f51043`,
`42a8c08`, `0291eb6`. Per the table model (R9), no row labels any commit as
the "current" head; exact current local/remote heads are reported in the
external handover at review time.

### R7 - Pin the privileged release workflow actions

Every action in `.github/workflows/release.yml` is pinned to a full commit SHA
resolved from the canonical upstream repositories, with the human-readable
version in a trailing comment (checkout v7.0.1, setup-go v7.0.0,
upload-artifact v7.0.1, download-artifact v8.0.1, attest-build-provenance
v4.2.2, softprops/action-gh-release v3.0.3). The single release job is split
into build (read-only) -> attest (id-token + attestations write only) ->
publish (contents write only), so no job holds more than it needs.
`scripts/test-release-contract.sh` records the pinned SHAs and fails on any
mutable action tag or unexpected SHA.

### R8 - Preserve contents: read in the attestation job

The attestation job previously overrode permissions with only
`id-token: write` and `attestations: write`, which drops the workflow-level
`contents: read` (job-level permissions do not inherit unspecified values).
The documented build-provenance contract requires
`contents: read`, `id-token: write` and `attestations: write`. The job now sets
exactly those three, and `test-release-contract.sh` requires exactly that set
and rejects any additional write permission. The build job stays
`contents: read`; the publish job stays limited to `contents: write`.

### R9 - Stop embedding a moving current head in the roadmap

The table's evidence column previously labelled earlier commits as the
"current local unpushed head", which went stale within the same commit series.
The model now records only immutable implementation/acceptance commits and
never names a "current" head; the repository state containing the ledger is
described as "this ledger commit and its ancestors", and the exact current
local/remote heads are reported in the external handover report at review time.
CP15 evidence: `b8d029e` (deterministic packaging), `a812642` (supported
environment constraint, scoped claim), `c698682` (pinned and separated release
workflow). CP21 preparation evidence: `5f51043` (release notes and runbook),
`42a8c08` (release-contract validation), `0291eb6` (rollback and tag-binding
corrections), plus this ledger's later repair sections. The R6 note and all
rows were audited for the same mistake.

### Hygiene

`scripts/package-release.sh` now has a terminating newline.

```text
Status: repaired locally; not pushed, tagged, published or announced
Verified: release-contract regression (attestation permission set);
  workflow YAML; reproducibility; test-release.sh; parity; GNU-tar fail-closed;
  roadmap commit-resolution audit
Remaining: tag and publication require a separate explicit review
```

## CP21 rehearsal - v0.1.0-rc.1 clean-machine release rehearsal

The real tag-triggered release path was exercised with `v0.1.0-rc.1`, without
promoting, announcing or marking anything stable.

- Prerelease semantics: `release.yml` now explicitly publishes a semver
  prerelease tag (`vX.Y.Z-*`) as a GitHub prerelease (computed from
  `GITHUB_REF_NAME`; the release action does not infer it), and the
  release-contract regression covers stable/prerelease detection. Pushed as
  commit `b616ebc`; CI passed; this is the tag target.
- Tag: annotated `v0.1.0-rc.1` created at `b616ebc`, `git rev-list -n 1`
  verified before push; no prior tag existed anywhere.
- Release workflow `33339028658` completed success; GitHub release created at
  `https://github.com/trestle-dev/trestle/releases/tag/v0.1.0-rc.1` with
  `prerelease: true`, `draft: false`; `/releases/latest` returns 404 (correctly
  excludes the prerelease). Seven assets uploaded (six archives + `SHA256SUMS`);
  all checksums verify; attestation job succeeded (pinned
  `attest-build-provenance` v4.2.2); all actions ran at their pinned SHAs.
- Integrity: all six archives contain exactly the versioned directory with
  `trestle`/`trestle.exe`, `README.md`, `LICENSE`, numeric ownership, no
  absolute/traversal paths, no source-tree debris or secrets; every binary
  embeds `0.1.0-rc.1` and `b616ebc...`; linux/amd64 natively reports
  `{"version":"0.1.0-rc.1","commit":"b616ebc...","os":"linux","arch":"amd64"}`.
- Clean-environment scripts against the public release: `download.sh`
  (pinned rc; cwd-only; checksum-verified; safe rerun; `--force`; unpinned
  `latest` correctly fails), `install.sh` (to `~/.local/bin`, mode 0755, PATH
  note; binary runs; unsupported OS/arch produce useful errors), `update.sh`
  (old install -> rc; data/config survive; rollback restores).
- SQLite first-run journey (public binary): health/ready, setupRequired,
  short-password 422, admin setup, sign-out/in, collection, records, realtime
  `record.created` events, audit events, restart persistence (3 records),
  backup -> destroy -> restore -> verify (3 records), wrong credentials 401.
- PostgreSQL first-run journey (PG 18.6): database/setup probe returns
  `restartRequired: true`; after restart provider is postgres, setup screen is
  administrator creation (not a login screen); admin setup, collection,
  records, audit events, restart persistence; wrong credentials 401; with the
  database stopped `/system/ready` returns `database_unavailable` while
  `/system/health` stays 200, and readiness recovers after restart.
- No secrets in either server log (password and URL scans: 0 occurrences).
- Website `https://trestle.cv`: HTTPS 200; `www` and `http` 301 to the apex;
  served `install.sh`/`download.sh`/`update.sh` byte-identical to canonical;
  install/quickstart/releases/updating/rollback/security/stability/first-run
  pages carry the required documentation (both install choices, `--system`,
  `--version v0.1.0`, SHA256SUMS/provenance, PostgreSQL restart guidance,
  backup/upgrade/rollback, known limitations, TLS responsibilities,
  mobile nav). Discrepancy recorded: no prerelease pinning example
  (e.g. `--version v0.1.0-rc.1`) appears in the docs; smallest repair is to add
  a prerelease pinning line to the install/releases pages.
- Provenance bundle retrieval via the unauthenticated attestations API returned
  404; the workflow attestation step itself succeeded (this is recorded as an
  evidence limitation, not a product failure).

```text
Status: rehearsal complete; release/tag preserved for independent review
Tag: v0.1.0-rc.1 at b616ebc (prerelease, excluded from latest)
Release: https://github.com/trestle-dev/trestle/releases/tag/v0.1.0-rc.1
Workflow: https://github.com/trestle-dev/trestle/actions/runs/33339028658
Recommendation: see the CP21 evidence report; no promotion or announcement
```

### Evidence classification (corrected)

- Workflow-step success: the attest job (pinned attest-build-provenance v4.2.2)
  completed successfully in run 33339028658.
- Authenticated provenance verification: NOT completed. `gh attestation verify`
  requires GitHub credentials; none are available in this environment and the
  device-flow login needs a human browser step. Investigation: the sigstore
  transparency log (rekor) has an entry whose subject digests match all six
  public archives (`/api/v1/index/retrieve` returned the same UUID for each),
  which is consistent with the attestation step having uploaded a bundle; the
  entry carries a signedEntryTimestamp and inclusionProof. Full cryptographic
  bundle verification is pending and is NOT claimed.
- Archive inspection: all six archives pass checksum, structure, ownership and
  path-safety inspection.
- Native Linux execution: linux/amd64 binary executed and reported
  `0.1.0-rc.1` / `b616ebc` / linux/amd64.
- Isolated host testing: the download/install/update and SQLite/PostgreSQL
  journeys ran in isolated directories with overridden HOME on the maintainer's
  Ubuntu host.
- Genuine container/VM testing: NOT performed (no container runtime/VM
  available in this environment); the true clean-machine rehearsal is
  incomplete.
- Unavailable native macOS and Windows testing: NOT performed (no native
  runtime); binaries inspected for embedded metadata only.
- Documentation repair: prerelease pinning examples
  (`--version v0.1.0-rc.1`) added to the website Install and Releases pages
  via Nift source; generated output rebuilt. Website source and output commits
  are separate.

## Checkpoint roadmap (authoritative status)

This table reconciles the public-preview campaign's actual accepted scope with
the earlier roadmap. Scopes that were previously double-counted or mislabelled
are corrected here; the original programme numbering (CHECKPOINTS.md CP00-CP23)
is a separate completed series and is not re-counted.

Model: the "evidence" column records only immutable implementation or
acceptance commits - never a "current local head", because a committed document
cannot reliably contain its own ever-changing future head. The exact current
local and remote heads are reported in the external handover report at review
time, not embedded here; the repository state containing this ledger is simply
"this ledger commit and its ancestors".

| Checkpoint | Canonical scope | Status | Evidence (immutable commits) | Remaining evidence |
| --- | --- | --- | --- | --- |
| CP1 | PostgreSQL contract and baseline | Complete, accepted | `04b6647` (historical commit) | none |
| CP2 | First-run PostgreSQL setup (ordinary-user) | Complete, accepted | `4c7534f` (historical commit) | none |
| CP3 | Schema and migration integrity, lineage, upgrades | Complete, accepted | `de37183` through CP3R3 (section evidence) | none |
| CP4 | Transactional mutation and audit atomicity (incl. fail-closed deletion) | Complete, accepted | `c8df51a` through CP4R2 (section evidence) | none |
| CP5 | Concurrency, locking and conflict behaviour | Complete, accepted | CP5/CP5R/CP5R2 sections (historical section evidence) | none |
| CP6 | Connection loss, restart and pool recovery | Complete, accepted | CP6 section (historical section evidence) | none |
| CP7 | Backup, restore and disaster recovery | Complete, accepted | CP7 section (historical section evidence) | none |
| CP8 | Longevity and resource bounds (soak) | Complete, accepted | CP8/CP8R sections (historical section evidence) | none |
| CP9 | SQLite/PostgreSQL parity matrix, provider-isolated evidence | Complete, accepted | CP9/CP9R/CP9R2 sections (historical section evidence) | none |
| CP10 | Authentication, authorization and session hardening | Complete, accepted | CP10/CP10R/CP10R2/CP10R3 sections (historical section evidence) | none |
| CP11 | Files, realtime, webhooks, jobs and functions hardening | Complete, accepted | CP11/CP11R/CP11R2/CP11R3 sections (historical section evidence) | none |
| CP12 | Degraded-state and operator UX (canonical scope corrected; the old roadmap title "import/export/deletion/upgrade compatibility" was not what CP12 delivered) | Complete, accepted | through CP12R5 `3e81aeb` | none |
| CP13 | Degraded-state and accessibility UX (old roadmap overlap with CP12) | Delivered within CP12; no separate preview checkpoint (reconciled, not double-counted) | n/a | none |
| CP14 | Independent example-application dogfood | Covered by the original programme (CHECKPOINTS.md CP21, DOGFOOD.md); not re-run in the preview ledger | n/a (inherited from the original programme) | none for preview scope |
| CP15 | Reproducible release artifacts | Complete (deterministic packaging + two-build regression) | `b8d029e` (deterministic packaging), `a812642` (supported-environment constraint, scoped claim), `c698682` (pinned and separated release workflow) | run the reproducible regression in CI |
| CP16 | Verified download/install/update and checksum public scripts (canonical scope corrected; uninstall not implemented) | Complete, accepted | accepted `e7a4972`; remote `main` `509be7b` (incl. CI repair) | uninstall path deliberately out of scope |
| CP17 | Container and service deployment | Deployment guidance from the original programme (CHECKPOINTS.md CP22, service/system docs); no official container image (documented) | n/a (inherited from the original programme) | container image remains a documented limitation |
| CP18 | Domain, HTTPS and reverse-proxy deployment | CNAME and live config recorded; live DNS/HTTPS not executed (explicitly deferred) | n/a | live deployment and verification pending |
| CP19 | Public-preview documentation and positioning | Website documentation complete; preview-status honesty maintained | website source `d9921ba` (accepted remote); generated output `4af2e41` (accepted remote) | final positioning review |
| CP20 | Launch assets and publication draft | LAUNCH.md draft exists; not published | n/a | publication draft review |
| CP21 | Clean-machine release rehearsal | Release-readiness prep complete (staged asset simulation + human runbook) | `5f51043` (release notes and runbook), `42a8c08` (release-contract validation), `0291eb6` (rollback and tag-binding corrections), plus this ledger's later repair sections | actual clean-machine rehearsal requires tag authorization |
| CP22 | Publish/no-publish review | Not done | n/a | requires explicit human review |
