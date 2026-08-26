#!/bin/sh
set -eu

go build -o "${TMPDIR:-/tmp}/trestle-dogfood-server" ./cmd/trestle
go build -o "${TMPDIR:-/tmp}/trestle-dogfood-seed" ./examples/incident-tracker/cmd/seed
data_dir=$(mktemp -d "${TMPDIR:-/tmp}/trestle-dogfood.XXXXXX")
restore_dir=$(mktemp -d "${TMPDIR:-/tmp}/trestle-restore.XXXXXX")
server_pid=""
cleanup() {
  if [ -n "$server_pid" ]; then
    kill "$server_pid" 2>/dev/null || true
    wait "$server_pid" 2>/dev/null || true
  fi
  rm -rf "$data_dir"
  rm -rf "$restore_dir"
}
trap cleanup EXIT INT TERM

"${TMPDIR:-/tmp}/trestle-dogfood-server" --data-dir "$data_dir" --listen 127.0.0.1:18090 >"$data_dir/server.log" 2>&1 &
server_pid=$!
"${TMPDIR:-/tmp}/trestle-dogfood-seed" -url http://127.0.0.1:18090

archive=$(find "$data_dir/backups" -maxdepth 1 -type f -name 'backup-*.zip' | head -n 1)
test -n "$archive"
kill "$server_pid"
wait "$server_pid" 2>/dev/null || true
server_pid=""
python3 -m zipfile -e "$archive" "$restore_dir"

"${TMPDIR:-/tmp}/trestle-dogfood-server" --data-dir "$restore_dir" --listen 127.0.0.1:18091 >"$restore_dir/server.log" 2>&1 &
server_pid=$!
"${TMPDIR:-/tmp}/trestle-dogfood-seed" -url http://127.0.0.1:18091 -verify-restored
