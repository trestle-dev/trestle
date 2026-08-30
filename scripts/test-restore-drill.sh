#!/bin/sh
# CP7 destructive backup / destroy / restore / verify drill.
#
# Runs a full recovery round trip against isolated temporary data only:
#   SQLite: start -> create data -> online backup -> destroy -> offline restore
#           into a new data directory -> verify
#   PostgreSQL: restore the portable archive into an empty initialized database
#           -> verify
# Requires curl, a Go toolchain and (for the PostgreSQL leg) initdb/pg_ctl
# (PGBINDIR).
set -eu

root=$(cd "$(dirname "$0")/.." && pwd)
bindir="${PGBINDIR:-}"
if [ -z "$bindir" ]; then
  for candidate in /usr/lib/postgresql/*/bin /usr/lib/postgresql/bin /usr/local/opt/postgresql*/bin; do
    if [ -x "$candidate/initdb" ]; then bindir="$candidate"; break; fi
  done
fi

work=$(mktemp -d "${TMPDIR:-/tmp}/trestle-restore.XXXXXX")
pid=""
trap '[ -z "$pid" ] || kill "$pid" 2>/dev/null || true; [ -n "${pgpid:-}" ] && kill "$pgpid" 2>/dev/null || true; rm -rf "$work"' EXIT INT TERM
cd "$root"
go build -o "$work/trestle" ./cmd/trestle

port=$((25400 + ($$ % 20000)))
base="http://127.0.0.1:$port"

start_server() {
  data_dir="$1"
  "$work/trestle" --listen "127.0.0.1:$port" --data-dir "$data_dir" >"$work/server.log" 2>&1 &
  pid=$!
  i=0
  until curl -fsS "$base/system/health" >/dev/null 2>&1; do
    i=$((i + 1)); [ "$i" -ge 80 ] && { cat "$work/server.log" >&2; exit 1; }; sleep 0.1
  done
}
stop_server() {
  [ -z "$pid" ] || { kill "$pid" 2>/dev/null || true; wait "$pid" 2>/dev/null || true; pid=""; }
}
setup_admin() {
  curl -fsS -c "$work/cj" -X POST "$base/admin/v1/setup" \
    -H 'Content-Type: application/json' -H "Origin: $base" \
    -d '{"email":"admin@example.com","password":"correct horse battery staple"}' \
    >"$work/setup.json"
  sed -E 's/.*"csrfToken":"([^"]+)".*/\1/' "$work/setup.json" | tr -d '\r\n' > "$work/csrf"
}
csrf() { cat "$work/csrf"; }

# ---- Leg 1: SQLite ----
start_server "$work/dataA"
setup_admin
curl -fsS -b "$work/cj" -X POST "$base/admin/v1/collections" \
  -H 'Content-Type: application/json' -H "Origin: $base" -H "X-Trestle-CSRF: $(csrf)" \
  -d '{"name":"issues","fields":[{"name":"title","type":"text","required":true}]}' >/dev/null
record=$(curl -fsS -b "$work/cj" -X POST "$base/api/v1/collections/issues/records" \
  -H 'Content-Type: application/json' -H "Origin: $base" -H "X-Trestle-CSRF: $(csrf)" \
  -d '{"values":{"title":"drill record"}}')
record_id=$(printf '%s' "$record" | sed -E 's/.*"id":"([^"]+)".*/\1/')
backup=$(curl -fsS -b "$work/cj" -X POST "$base/admin/v1/backups" \
  -H 'Content-Type: application/json' -H "Origin: $base" -H "X-Trestle-CSRF: $(csrf)" -d '{}')
backup_id=$(printf '%s' "$backup" | sed -E 's/.*"id":"([^"]+)".*/\1/')
curl -fsS -b "$work/cj" "$base/admin/v1/backups/$backup_id" -o "$work/archive.zip"
stop_server
rm -rf "$work/dataA"
"$work/trestle" restore --backup "$work/archive.zip" --data-dir "$work/dataB" >/dev/null
start_server "$work/dataB"
# Sessions are revoked by the restore policy; sign in again and verify.
curl -fsS -c "$work/cj2" -X POST "$base/admin/v1/session" \
  -H 'Content-Type: application/json' -H "Origin: $base" \
  -d '{"email":"admin@example.com","password":"correct horse battery staple"}' >/dev/null
curl -fsS -b "$work/cj2" "$base/api/v1/collections/issues/records/$record_id" \
  | grep -q '"drill record"' || { echo "SQLite restore verification failed" >&2; exit 1; }
stop_server
echo "1) SQLite backup -> destroy -> restore -> verify passed"

# ---- Leg 2: PostgreSQL ----
if [ -n "$bindir" ] && [ -x "$bindir/initdb" ]; then
  pgport=$((27400 + ($$ % 20000)))
  "$bindir/initdb" -D "$work/pg" -U postgres --auth=trust --no-locale -E UTF8 >"$work/initdb.log" 2>&1
  attempt=1
  while [ "$attempt" -le 3 ]; do
    p=$((pgport + (attempt - 1) * 97))
    if "$bindir/pg_ctl" -D "$work/pg" -l "$work/pg.log" -o "-p $p -k $work" start >/dev/null 2>&1; then
      pgport="$p"
      break
    fi
    echo "postgres start attempt $attempt on port $p failed:" >&2
    cat "$work/pg.log" >&2 2>/dev/null || true
    attempt=$((attempt + 1))
    [ "$attempt" -gt 3 ] && { echo "could not start disposable PostgreSQL" >&2; exit 1; }
  done
  "$bindir/createdb" -h 127.0.0.1 -p "$pgport" -U postgres trestle_restore
  pgpid=$("$bindir/pg_ctl" -D "$work/pg" status | sed -n 's/.*PID: \([0-9]*\).*/\1/p')
  pgurl="postgres://postgres@127.0.0.1:$pgport/trestle_restore?sslmode=disable"
  # The restore contract requires an initialized empty destination at the
  # current schema; start Trestle against the raw database to initialize it.
  "$work/trestle" --listen "127.0.0.1:$port" --data-dir "$work/dataPg" \
    --database-provider postgres --database-url "$pgurl" >"$work/init-pg.log" 2>&1 &
  pid=$!
  i=0
  until curl -fsS "$base/system/health" >/dev/null 2>&1; do
    i=$((i + 1)); [ "$i" -ge 80 ] && { cat "$work/init-pg.log" >&2; exit 1; }; sleep 0.1
  done
  stop_server
  "$work/trestle" restore --backup "$work/archive.zip" --provider postgres --database-url "$pgurl" >/dev/null
  "$work/trestle" --listen "127.0.0.1:$port" --data-dir "$work/dataPg" \
    --database-provider postgres --database-url "$pgurl" >"$work/server-pg.log" 2>&1 &
  pid=$!
  i=0
  until curl -fsS "$base/system/health" >/dev/null 2>&1; do
    i=$((i + 1)); [ "$i" -ge 80 ] && { cat "$work/server-pg.log" >&2; exit 1; }; sleep 0.1
  done
  curl -fsS -c "$work/cj3" -X POST "$base/admin/v1/session" \
    -H 'Content-Type: application/json' -H "Origin: $base" \
    -d '{"email":"admin@example.com","password":"correct horse battery staple"}' >/dev/null
  curl -fsS -b "$work/cj3" "$base/api/v1/collections/issues/records/$record_id" \
    | grep -q '"drill record"' || { echo "PostgreSQL restore verification failed" >&2; exit 1; }
  stop_server
  echo "2) portable archive restore into empty PostgreSQL -> verify passed"
else
  echo "2) PostgreSQL leg skipped (no initdb/pg_ctl found)"
fi

echo "restore drill passed"