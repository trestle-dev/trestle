#!/bin/sh
# PostgreSQL-focused readiness gate for the public-preview campaign (CP1).
#
# Proves a real PostgreSQL server inside the declared supported window runs the
# provider-parameterized suite. No mock or SQL parser is used: either a real
# server is provided through TRESTLE_TEST_POSTGRES_URL, or this script
# provisions a disposable local instance with initdb/pg_ctl and destroys it on
# exit.
#
# Optional environment:
#   TRESTLE_TEST_POSTGRES_URL  reuse an existing server (postgres://...)
#   PGBINDIR                   directory containing initdb/pg_ctl/psql
#   TRESTLE_PG_PORT            port for the disposable server (default: dynamic)
#
# Usage:
#   ./scripts/test-postgres-gate.sh
#
# The gate never prints the connection URL, so credentials cannot leak into CI
# logs. It prints only whether the server was provided or provisioned and the
# server version reported by the contract test.

set -eu

url="${TRESTLE_TEST_POSTGRES_URL:-}"
provisioned=0
root=$(cd "$(dirname "$0")/.." && pwd)
bindir="${PGBINDIR:-}"

if [ -z "$bindir" ]; then
  for candidate in \
    /usr/lib/postgresql/*/bin \
    /usr/lib/postgresql/bin \
    /usr/local/opt/postgresql*/bin
  do
    if [ -x "$candidate/initdb" ]; then
      bindir="$candidate"
      break
    fi
  done
fi

cleanup() {
  if [ "$provisioned" = 1 ]; then
    "$bindir/pg_ctl" -D "$pgdata" stop -m fast >/dev/null 2>&1 || true
    rm -rf "${tmpdir:?}"
  fi
}
trap cleanup EXIT INT TERM

if [ -n "$url" ]; then
  echo "PostgreSQL gate: using provided TRESTLE_TEST_POSTGRES_URL"
elif [ -n "$bindir" ] && [ -x "$bindir/initdb" ]; then
  if [ "$(id -u)" = 0 ]; then
    echo "PostgreSQL gate: refusing to provision a server as root; initdb cannot run as root" >&2
    exit 1
  fi
  tmpdir=$(mktemp -d "${TMPDIR:-/tmp}/trestle-pg-gate.XXXXXX")
  pgdata="$tmpdir/data"
  port="${TRESTLE_PG_PORT:-$((25432 + ($$ % 20000)))}"
  echo "PostgreSQL gate: provisioning disposable local instance on 127.0.0.1:$port"
  "$bindir/initdb" -D "$pgdata" -U postgres --auth=trust --no-locale -E UTF8 >"$tmpdir/initdb.log" 2>&1
  "$bindir/pg_ctl" -D "$pgdata" -l "$tmpdir/server.log" -o "-p $port -k $tmpdir" start >/dev/null
  "$bindir/createdb" -h 127.0.0.1 -p "$port" -U postgres trestle_test
  url="postgres://postgres@127.0.0.1:$port/trestle_test?sslmode=disable"
  provisioned=1
else
  echo "PostgreSQL gate: no TRESTLE_TEST_POSTGRES_URL and no PostgreSQL binaries found" >&2
  echo "Set TRESTLE_TEST_POSTGRES_URL or PGBINDIR to run this gate." >&2
  exit 1
fi

export TRESTLE_TEST_POSTGRES_URL="$url"

cd "$root"

echo "PostgreSQL gate: contract and version-window probe"
go test ./internal/store -run 'TestPostgresReadinessContract' -v

echo "PostgreSQL gate: full provider-parameterized suite"
go test ./...

echo "PostgreSQL gate: race-enabled provider suite"
go test -race ./...

echo "PostgreSQL gate: passed against a real server"