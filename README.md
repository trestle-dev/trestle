# Trestle

Trestle is a planned open-source, self-hosted application backend with
collections, authentication, access rules, files, realtime APIs, migrations,
administration, backups, and event-driven integrations in one compact binary.

Trestle is not a PocketBase API clone. Its stable HTTP/JSON/SSE contracts are
intended to work from browsers, mobile applications, and trusted backends
written in any language.

The repository is currently pre-release. Checkpoints CP00–CP05 are implemented:
the server lifecycle, SQLite migrations, embedded administration shell, secure
first-run administrator sessions, and base-collection metadata are working.
Physical collection tables and record CRUD begin at CP06–CP07.

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
currently require at least 12 characters.

Trestle creates `./data/trestle.db` by default. The data directory is set to
owner-only permissions and must live on a local filesystem; shared/network
filesystem operation is not supported.

## Configuration

Flags override environment variables, which override defaults:

| Flag | Environment | Default |
| --- | --- | --- |
| `--listen` | `TRESTLE_LISTEN` | `127.0.0.1:8090` |
| `--data-dir` | `TRESTLE_DATA_DIR` | `./data` |
| `--shutdown-timeout` | `TRESTLE_SHUTDOWN_TIMEOUT` | `10s` |
| `--log-level` | `TRESTLE_LOG_LEVEL` | `info` |
| `--static-dir` | `TRESTLE_STATIC_DIR` | embedded assets |

The static-directory override is intended for frontend development. For
example, after running Nift in another terminal:

```sh
go run ./cmd/trestle --static-dir internal/web/public
```

Trestle logs a warning whenever the override is active.

## Verify a change

```sh
nift build
nift status
node --check content/assets/js/script.js
node scripts/check-web.mjs internal/web/public
go test ./...
go test -race ./...
go vet ./...
```

CI additionally compiles Linux, macOS, and Windows targets for both amd64 and
arm64.

## Current safety boundary

Trestle has no supported release yet. Keep development instances on loopback or
a trusted private network. Reverse-proxy, TLS, upgrade, backup, and stable-release
guidance arrives in later checkpoints; the current schema is not stable.

## Project documents

Start with:

- [HANDOVER.md](HANDOVER.md) — product invariants and engineering workflow
- [CHECKPOINTS.md](CHECKPOINTS.md) — step-by-step implementation handover
- [PLAN.md](PLAN.md) — ordered implementation programme
- [ARCHITECTURE.md](ARCHITECTURE.md) — system and persistence boundaries
- [API.md](API.md) — language-neutral API contract direction
- [UI.md](UI.md) — fixed-viewport vanilla dashboard contract
- [FUNCTIONS.md](FUNCTIONS.md) — AWS Lambda event integration
- [SECURITY.md](SECURITY.md) — trust model and security invariants
