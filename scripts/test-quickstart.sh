#!/bin/sh
set -eu

root=$(mktemp -d "${TMPDIR:-/tmp}/trestle-quickstart.XXXXXX")
pid=""
trap '[ -z "$pid" ] || kill "$pid" 2>/dev/null || true; rm -rf "$root"' EXIT INT TERM
port=$((20000 + ($$ % 20000)))
go build -trimpath -o "$root/trestle" ./cmd/trestle
"$root/trestle" --listen "127.0.0.1:$port" --data-dir "$root/data" >"$root/server.log" 2>&1 &
pid=$!
i=0
until curl -fsS "http://127.0.0.1:$port/system/health" >/dev/null 2>&1; do
  i=$((i + 1))
  if [ "$i" -ge 50 ]; then cat "$root/server.log" >&2; exit 1; fi
  sleep 0.1
done
curl -fsS "http://127.0.0.1:$port/system/ready" | grep -q '"status":"ready"'
curl -fsS "http://127.0.0.1:$port/system/version" | grep -q '"version"'
curl -fsS "http://127.0.0.1:$port/" | grep -q 'Trestle'
[ "$(stat -c %a "$root/data")" = 700 ]
echo "clean quickstart passed"
