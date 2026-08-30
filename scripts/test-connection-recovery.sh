#!/bin/sh
# CP6 connection-loss / recovery drill against a real disposable PostgreSQL.
#
# Starts Trestle against a real PostgreSQL instance, stops the database,
# verifies /system/ready reports database_unavailable while /system/health
# stays 200 (process liveness), restarts the database, and verifies readiness
# recovers. Requires initdb/pg_ctl (PGBINDIR) and an unprivileged user.
set -eu

root=$(cd "$(dirname "$0")/.." && pwd)
bindir="${PGBINDIR:-}"
if [ -z "$bindir" ]; then
  for candidate in /usr/lib/postgresql/*/bin /usr/lib/postgresql/bin /usr/local/opt/postgresql*/bin; do
    if [ -x "$candidate/initdb" ]; then bindir="$candidate"; break; fi
  done
fi
if [ -z "$bindir" ] || [ ! -x "$bindir/initdb" ]; then
  echo "no PostgreSQL binaries found; set PGBINDIR" >&2
  exit 1
fi
if [ "$(id -u)" = 0 ]; then echo "refusing to run as root" >&2; exit 1; fi

work=$(mktemp -d "${TMPDIR:-/tmp}/trestle-conn.XXXXXX")
pid=""
trap '[ -z "$pid" ] || kill "$pid" 2>/dev/null || true; [ -n "${pgpid:-}" ] && kill "$pgpid" 2>/dev/null || true; rm -rf "$work"' EXIT INT TERM

port=$((26432 + ($$ % 20000)))
pgdata="$work/pg"
"$bindir/initdb" -D "$pgdata" -U postgres --auth=trust --no-locale -E UTF8 >"$work/initdb.log" 2>&1

# start_pg starts a disposable instance with a bounded retry on a fresh port;
# a failed attempt prints the PostgreSQL log and the chosen port rather than
# silently normalising an infrastructure flake.
start_pg() {
  attempt=1
  while [ "$attempt" -le 3 ]; do
    p=$((port + (attempt - 1) * 97))
    if "$bindir/pg_ctl" -D "$pgdata" -l "$work/pg.log" -o "-p $p -k $work" start >/dev/null 2>&1; then
      port="$p"
      return 0
    fi
    echo "postgres start attempt $attempt on port $p failed:" >&2
    cat "$work/pg.log" >&2 2>/dev/null || true
    attempt=$((attempt + 1))
  done
  echo "could not start disposable PostgreSQL" >&2
  exit 1
}
start_pg
"$bindir/createdb" -h 127.0.0.1 -p "$port" -U postgres trestle
pgpid=$("$bindir/pg_ctl" -D "$work/pg" status | sed -n 's/.*PID: \([0-9]*\).*/\1/p')
url="postgres://postgres@127.0.0.1:$port/trestle?sslmode=disable"
appport=$((24400 + ($$ % 20000)))
base="http://127.0.0.1:$appport"

cd "$root"
go build -o "$work/trestle" ./cmd/trestle
"$work/trestle" --listen "127.0.0.1:$appport" --data-dir "$work/data" \
  --database-provider postgres --database-url "$url" --database-connect-timeout 5s \
  >"$work/server.log" 2>&1 &
pid=$!

i=0
until curl -fsS "$base/system/health" >/dev/null 2>&1; do
  i=$((i + 1)); [ "$i" -ge 80 ] && { cat "$work/server.log" >&2; exit 1; }; sleep 0.1
done
i=0
until [ "$(curl -s -o /dev/null -w '%{http_code}' "$base/system/ready")" = 200 ]; do
  i=$((i + 1)); [ "$i" -ge 80 ] && { cat "$work/server.log" >&2; exit 1; }; sleep 0.1
done
echo "1) ready 200 with database up"

# Stop the database; readiness must report database_unavailable promptly and
# liveness (/system/health) must remain 200.
"$bindir/pg_ctl" -D "$work/pg" stop -m fast >/dev/null
pgpid=""
start=$(date +%s)
until [ "$(curl -s -o /dev/null -w '%{http_code}' "$base/system/ready")" = 503 ]; do
  [ "$(($(date +%s) - start))" -ge 20 ] && { echo "ready did not become 503" >&2; exit 1; }
  sleep 0.2
done
if ! curl -s "$base/system/ready" | grep -q database_unavailable; then
  echo "ready 503 did not report database_unavailable" >&2
  exit 1
fi
[ "$(curl -s -o /dev/null -w '%{http_code}' "$base/system/health")" = 200 ] || { echo "health not 200 during outage" >&2; exit 1; }
echo "2) database_unavailable (503) with health 200 while database down"

# Restart the database; readiness must recover.
# Restart the database on the same port; a bounded wait with the log printed
# on failure.
i=0
until "$bindir/pg_ctl" -D "$work/pg" -l "$work/pg.log" -o "-p $port -k $work" start >/dev/null 2>&1; do
  i=$((i + 1))
  if [ "$i" -ge 5 ]; then
    echo "postgres restart on port $port failed:" >&2
    cat "$work/pg.log" >&2 2>/dev/null || true
    exit 1
  fi
  sleep 1
done
pgpid=$("$bindir/pg_ctl" -D "$work/pg" status | sed -n 's/.*PID: \([0-9]*\).*/\1/p')
i=0
until [ "$(curl -s -o /dev/null -w '%{http_code}' "$base/system/ready")" = 200 ]; do
  i=$((i + 1)); [ "$i" -ge 100 ] && { echo "readiness did not recover" >&2; exit 1; }; sleep 0.2
done
echo "3) ready 200 after database recovery"

# No credentials in server logs.
if grep -q "postgres://" "$work/server.log"; then
  echo "connection material leaked in logs" >&2
  exit 1
fi
echo "connection-loss/recovery drill passed"