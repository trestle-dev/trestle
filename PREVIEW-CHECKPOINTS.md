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

## Checkpoint roadmap

- CP1 - PostgreSQL contract and baseline (this checkpoint)
- CP2 - Repair first-run PostgreSQL setup (ordinary-user workflow)
- CP3 - Schema and migration integrity
- CP4 - Transactional mutation and audit atomicity
- CP5 - Concurrency, locking and conflict behaviour
- CP6 - Connection loss, restart and pool recovery
- CP7 - Backup, restore and disaster recovery
- CP8 - Longevity and resource bounds
- CP9 - SQLite/PostgreSQL parity matrix
- CP10 - Authentication, authorization and session hardening
- CP11 - Files, realtime, webhooks, jobs and functions
- CP12 - Import, export, deletion and upgrade compatibility
- CP13 - Degraded-state and accessibility UX
- CP14 - Independent example-application dogfood
- CP15 - Reproducible release artifacts
- CP16 - Public install.sh, update and uninstall paths
- CP17 - Container and service deployment
- CP18 - Domain, HTTPS and reverse-proxy deployment
- CP19 - Public-preview documentation and positioning
- CP20 - Launch assets and publication draft
- CP21 - Clean-machine release rehearsal
- CP22 - Publish/no-publish review