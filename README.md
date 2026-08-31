# Trestle

Trestle is an open-source, self-hosted backend platform with
collections, authentication, access rules, files, realtime APIs, migrations,
administration, backups, and event-driven integrations in one compact binary.

Its stable HTTP/JSON/SSE contracts are intended to work from browsers, mobile
applications, and trusted backends written in any language.

For a complete external application, see
[Incident Desk](https://github.com/trestle-dev/trestle-example). It combines
application users, collection rules, typed records, realtime SSE and files
without importing Trestle packages or opening the SQLite database.

The repository is currently a release candidate. Checkpoints CP00-CP22 and all
ten CP23 hardening campaigns are implemented,
covering the process and SQLite foundation, embedded administration, typed
collections and records, authentication and access rules, local and
S3-compatible files, realtime events, audit, durable jobs, webhooks, AWS Lambda,
OpenAPI, reference clients, backup/recovery, and whole-product dogfooding.
Deployment, release automation and the release-candidate matrix are implemented.

A separate PostgreSQL parity campaign (PG00-PG11) is complete. PostgreSQL is an
available external-database option for deployments needing its operational
model; SQLite remains the embedded zero-configuration option. The parity
corpus, migration safety and recovery semantics are exercised against
PostgreSQL 16, 17 and 18 in CI and PostgreSQL 18.6 locally. See
`POSTGRES-CHECKPOINTS.md`.

An active public-preview campaign (`PREVIEW-CHECKPOINTS.md`) is hardening the
whole product for an honest preview. It adds a machine-readable PostgreSQL
readiness contract, a single reproducible PostgreSQL gate
(`scripts/test-postgres-gate.sh`), and proceeds through recovery, parity,
release, domain and publish/no-publish checkpoints. Trestle is an early preview
for evaluation and prototypes; it is not presented as battle-proven or
production-immune. Nothing is published until the campaign's publish gate
authorizes it.

## Requirements

- Go 1.22 or newer
- A current Nift 4.x executable available as `nift`

Nift is a build-time dependency only. The generated dashboard is embedded in
the Go executable; deployments do not need Nift or a separate static directory.

## Build from source

Clone the repository, then build the Nift frontend before compiling Go:

```sh
nift build
nift status
go build -o trestle ./cmd/trestle
```

`nift status` should report all three tracked application outputs current. Nift
writes them beneath `internal/web/public/`, where Go's `embed` package includes
them in the executable. After changing `content/`, `templates/`, or frontend
assets, run `nift build` and rebuild the Go binary.

## Run Trestle

Start the compiled binary:

```sh
./trestle
```

Or build and run directly during development:

```sh
nift build
go run ./cmd/trestle
```

The default address is:

```text
http://127.0.0.1:8090
```

On first run, open that address and create the first administrator. The setup
route closes once the administrator is committed. Administrator passwords
currently require at least 7 characters.

First-run setup offers SQLite and PostgreSQL. **SQLite is the embedded
zero-configuration option**; PostgreSQL is the available external-database
option for deployments needing a server database. The setup selector persists
the chosen provider in an owner-only configuration file.

Trestle creates `./data/trestle.db` by default. The data directory is set to
owner-only permissions and must live on a local filesystem; shared/network
filesystem operation is not supported.

## Run as a systemd user service

Run Trestle in the foreground with `./trestle` or `trestle serve`. To keep it
running without a terminal, install a per-user systemd unit:

```sh
trestle service install                 # optional --listen, --data-dir, --env-file
trestle service status
trestle service logs                # or: trestle service logs --follow
trestle service restart
trestle service uninstall           # stops the service but keeps Trestle data
```

The user unit is written to `~/.config/systemd/user/trestle.service` and managed
with `systemctl --user` and `journalctl --user-unit trestle.service`. `service
install` resolves the executable to a stable absolute path, refuses empty,
relative or transient paths, writes the unit atomically, reloads systemd, and
enables and starts the service. An existing unit that is not managed by Trestle
is never overwritten or removed silently. `trestle service status` reports
enabled/running state, PID, version, listen address and a live health check of
the public `GET /system/health` endpoint, and exits nonzero when the service is
failed or missing.

`service install` records `--listen` and `--data-dir` (default `./data` made
absolute) in the unit. Because a systemd user service does not inherit the
shell environment, use an explicit protected environment file for the
remaining `TRESTLE_*` configuration (including `TRESTLE_DATABASE_URL` for
PostgreSQL and `TRESTLE_S3_*`/`TRESTLE_AWS_*` credentials):

```sh
trestle service install --env-file /absolute/protected/trestle.env
```

The file must be an absolute, regular, non-symlink file with exactly `0600`
permissions, owned by the invoking user; it is referenced by the unit's
`EnvironmentFile=` and its path is recorded in the integrity-checked managed
metadata. Secret values are never copied into the unit or printed. The recorded
environment file is revalidated before `start`, `restart` and `status`; `stop`,
`logs` and `uninstall` remain available even if it is missing. Changing the file
takes effect on `trestle service restart`. Install creates the data directory
with owner-only permissions and refuses symlink, non-directory or
group/world-writable data paths. Repeated `service install` calls preserve the
installed listen, data directory and environment file unless a flag is given
explicitly. `service install --system` (system-wide units) is a documented
follow-up and is not yet supported; user mode is the default.

## Install a release

Two verified download paths are available. Both fetch the exact release archive
and its `SHA256SUMS`, verify the archive before writing anything, and never
execute the downloaded binary.

### Install on this user account

Writes to `~/.local/bin` (machine-wide `--system` installations are explicit):

```sh
curl -fsSL https://trestle.cv/install.sh | sh
```

### Download into this directory

Writes only `./trestle` in the current directory and changes nothing else -
suitable for portable folders, testing, manually operated servers, and users
who want to inspect exactly where the binary lives:

```sh
curl -fsSL https://trestle.cv/download.sh | sh
./trestle
```

`download.sh` supports a pinned version, an explicit output path, and `--force`
to overwrite an existing file:

```sh
curl -fsSL https://trestle.cv/download.sh | sh -s -- --version v0.1.0
curl -fsSL https://trestle.cv/download.sh | sh -s -- --output ./some-name
```

It refuses to overwrite an existing file, directory or symlink unless `--force`
is passed. Downloading never requires root.

### Update

Update the installed binary without changing instance data or configuration:

```sh
curl -fsSL https://trestle.cv/update.sh | sh
```

The updater supports `--dry-run`, `--version vX.Y.Z`, and `--rollback`, verifies
the selected archive with the same portable checksum check, and retains
`trestle.previous` for rollback. System installations pass `--system` through
`sudo` for install and update:

```sh
curl -fsSL https://trestle.cv/install.sh | sudo sh -s -- --system
```

All three public scripts verify archives with `sha256sum` where available and
fall back to `shasum -a 256` on macOS. Tagged releases contain Linux, macOS,
and Windows archives for amd64 and arm64, plus checksums and build provenance.

## Configuration

Flags override environment variables, which override defaults:

| Flag | Environment | Default |
| --- | --- | --- |
| `--listen` | `TRESTLE_LISTEN` | `127.0.0.1:8090` |
| `--data-dir` | `TRESTLE_DATA_DIR` | `./data` |
| `--shutdown-timeout` | `TRESTLE_SHUTDOWN_TIMEOUT` | `10s` |
| `--log-level` | `TRESTLE_LOG_LEVEL` | `info` |
| `--static-dir` | `TRESTLE_STATIC_DIR` | embedded assets |
| `--trusted-proxies` | `TRESTLE_TRUSTED_PROXIES` | none |
| `--read-header-timeout` | `TRESTLE_READ_HEADER_TIMEOUT` | `5s` |
| `--read-timeout` | `TRESTLE_READ_TIMEOUT` | `5m` |
| `--idle-timeout` | `TRESTLE_IDLE_TIMEOUT` | `60s` |
| `--max-header-bytes` | `TRESTLE_MAX_HEADER_BYTES` | `1048576` |
| `--database-provider` | `TRESTLE_DATABASE_PROVIDER` | `sqlite` |
| `--database-url` | `TRESTLE_DATABASE_URL` | none (PostgreSQL only) |
| `--database-max-open` | `TRESTLE_DATABASE_MAX_OPEN` | `10` |
| `--database-max-idle` | `TRESTLE_DATABASE_MAX_IDLE` | `2` |
| `--database-connect-timeout` | `TRESTLE_DATABASE_CONNECT_TIMEOUT` | `10s` |
| `--database-conn-max-lifetime` | `TRESTLE_DATABASE_CONN_MAX_LIFETIME` | `30m` |

### Database providers

- `sqlite` (default) is the complete provider: one process owns one local
  database at `<data-dir>/trestle.db` with WAL, owner-only permissions, and
  refused unknown future schemas.
- `postgres` is available. Explicit startup configuration
  (`--database-provider postgres --database-url postgres://...`) or the stored
  first-run bootstrap selects it. Remote connections require TLS unless the
  host is loopback. The URL is stored atomically in a `0600`
  `<data-dir>/database.json` and is never returned by an API or written to
  logs, diagnostics, or support bundles.
- `--database-connect-timeout` must be a whole number of seconds. Trestle
  injects it as the driver's `connect_timeout` into every PostgreSQL connection
  configuration (startup, stored bootstrap, explicit flags, and the first-run
  connection test) so a hanging peer cannot stall startup indefinitely.
- The applied schema version is derived from validated
  `_trestle_schema_migrations` history on both providers. On SQLite,
  `PRAGMA user_version` is a compatibility mirror: a valid history restores an
  absent mirror, but history is never reconstructed from the marker, and
  disagreement or damaged history fails closed.
- Interrupted first-run setup is resumable: provider selection persists before
  restart, administrator creation stays available until one administrator
  commits, and setup never reopens afterward. A persisted but unreachable
  PostgreSQL configuration fails startup with a redacted error and requires
  manual `database.json` recovery.

The static-directory override is intended for frontend development. For
example, after running Nift in another terminal:

```sh
go run ./cmd/trestle --static-dir internal/web/public
```

Trestle logs a warning whenever the override is active.

Forwarded client and scheme headers are ignored unless the immediate peer is
inside an explicitly configured trusted-proxy CIDR. Bind Trestle to loopback
behind Caddy or nginx, preserve the public `Host`, and configure only the exact
proxy addresses you administer.

## Verify a change

```sh
nift build
nift status
node --check internal/web/public/assets/js/script.js
node scripts/check-web.mjs internal/web/public
go test ./...
go test -race ./...
go vet ./...
./scripts/test-release.sh
```

CI additionally compiles Linux, macOS, and Windows targets for both amd64 and
arm64.

## Current safety boundary

Trestle has no published stable release yet. CP23 is complete and its support
boundary is recorded in `STABILITY.md`; a tagged stable release and independent
field evidence remain distinct from completing the engineering campaign.

## Project documents

Start with:

- [HANDOVER.md](HANDOVER.md) - product invariants and engineering workflow
- [CHECKPOINTS.md](CHECKPOINTS.md) - step-by-step implementation handover
- [PLAN.md](PLAN.md) - ordered implementation programme
- [ARCHITECTURE.md](ARCHITECTURE.md) - system and persistence boundaries
- [API.md](API.md) - language-neutral API contract direction
- [UI.md](UI.md) - fixed-viewport vanilla dashboard contract
- [FUNCTIONS.md](FUNCTIONS.md) - AWS Lambda event integration
- [SECURITY.md](SECURITY.md) - trust model and security invariants
