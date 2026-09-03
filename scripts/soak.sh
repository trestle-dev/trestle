#!/bin/sh
# CP8 sustained-workload soak with resource-growth bounds.
#
# Runs a warm, sustained CRUD/auth/realtime/files/backup workload against one
# Trestle process, samples the SAME process periodically (baseline, periodic,
# peak, final settled), and fails if file-descriptor, thread or settled-RSS
# growth exceeds documented bounds. A separate phase restarts the process and
# verifies recovery (records, file metadata, backup visibility, session
# behavior); the restarted process is NOT used in the leak comparison. The
# script is a reproducibility and regression instrument, not a marketing
# benchmark: numbers are this-machine-only.
#
# Bounds (generous, explicit, documented):
#   fd growth      <= 25 file descriptors
#   thread growth  <= 20 threads
#   settled RSS growth <= 50 MiB (51200 KiB); heap-live retention is measured
#   separately via GODEBUG=gctrace so a leak is distinguished from RSS-level
#   allocator retention
#
# Usage: SOAK_SECONDS=120 ./scripts/soak.sh    (default 60)
set -eu

root=$(cd "$(dirname "$0")/.." && pwd)
seconds=${SOAK_SECONDS:-60}
work=$(mktemp -d "${TMPDIR:-/tmp}/trestle-soak.XXXXXX")
pid=""
trap '[ -z "$pid" ] || kill "$pid" 2>/dev/null || true; rm -rf "$work"' EXIT INT TERM

cd "$root"
go build -o "$work/trestle" ./cmd/trestle
port=$((28400 + ($$ % 20000)))
base="http://127.0.0.1:$port"

GODEBUG=gctrace=1 "$work/trestle" --listen "127.0.0.1:$port" --data-dir "$work/data" >"$work/server.log" 2>&1 &
pid=$!
i=0
until curl -fsS "$base/system/health" >/dev/null 2>&1; do
  i=$((i + 1)); [ "$i" -ge 80 ] && { cat "$work/server.log" >&2; exit 1; }; sleep 0.1
done

setup() {
  curl -fsS -c "$work/cj" -X POST "$base/admin/v1/setup" \
    -H 'Content-Type: application/json' -H "Origin: $base" \
    -d '{"email":"admin@example.com","password":"correct horse battery staple","applicationRegistrationPolicy":"closed"}' >/dev/null
  token=$(curl -fsS -b "$work/cj" -c "$work/cj" "$base/admin/v1/session" \
    | sed -E 's/.*"csrfToken":"([^"]+)".*/\1/' | tr -d '\r\n')
  curl -fsS -b "$work/cj" -X POST "$base/admin/v1/collections" \
    -H 'Content-Type: application/json' -H "Origin: $base" -H "X-Trestle-CSRF: $token" \
    -d '{"name":"soak","fields":[{"name":"title","type":"text","required":true},{"name":"n","type":"number"}]}' >/dev/null
  # Webhook target exists only to prove enqueue activity; delivery to a
  # private/loopback destination is refused by the SSRF guard by design.
  curl -fsS -b "$work/cj" -X POST "$base/admin/v1/webhooks" \
    -H 'Content-Type: application/json' -H "Origin: $base" -H "X-Trestle-CSRF: $token" \
    -d "{\"name\":\"sink\",\"url\":\"https://receiver.invalid/deliver\",\"topics\":[\"record.created\"]}" >/dev/null
  printf '%s' "$token" > "$work/csrf"
}
setup

sample() {
  pid="$1"
  vms=$(awk '/VmRSS:/{print $2}' "/proc/$pid/status" 2>/dev/null || echo 0)
  fds=$(ls /proc/$pid/fd 2>/dev/null | wc -l)
  thr=$(awk '/Threads:/{print $2}' "/proc/$pid/status" 2>/dev/null || echo 0)
  printf '%s %s %s' "$vms" "$fds" "$thr"
}
# heapSample parses the most recent Go GC line from the server log
# (GODEBUG=gctrace) into "gc_count heap_live_MB elapsed_s".
heap_sample() {
  line=$(grep -oE 'gc [0-9]+ @[0-9.]+s' "$work/server.log" 2>/dev/null | tail -1)
  gccount=0; elapsed=0
  if [ -n "$line" ]; then
    set -- $line
    gccount=${2:-0}
    elapsed=$(printf '%s' "$3" | tr -d '@s')
  fi
  live=$(grep -oE '[0-9.]+->[0-9.]+->[0-9.]+ MB' "$work/server.log" 2>/dev/null | tail -1 | sed -E 's/.*->([0-9.]+) MB/\1/')
  [ -z "$live" ] && live=0
  printf '%s %s %s' "$gccount" "$live" "$elapsed"
}

# Warm the process so the baseline is the settled steady state, not startup
# peak (startup loads the runtime and embedded assets).
warm=40
for n in $(seq 1 "$warm"); do
  curl -fsS -o /dev/null -b "$work/cj" -X POST "$base/api/v1/collections/soak/records" \
    -H 'Content-Type: application/json' -H "Origin: $base" -H "X-Trestle-CSRF: $(cat "$work/csrf")" \
    -d "{\"values\":{\"title\":\"warm$n\",\"n\":$n}}" || true
done
sleep 1
before=$(sample "$pid")
heap_before=$(heap_sample)

# Sustained CRUD with latency sampling on the same process.
created=0; errors=0
total_ms=0; max_ms=0; samples=0
peak_fds=0; peak_thr=0; peak_rss=0
start=$(date +%s)
last_sample=$start
while [ "$(($(date +%s) - start))" -lt "$seconds" ]; do
  t0=$(date +%s%N)
  code=$(curl -s -o /dev/null -w '%{http_code}' -b "$work/cj" -X POST "$base/api/v1/collections/soak/records" \
    -H 'Content-Type: application/json' -H "Origin: $base" -H "X-Trestle-CSRF: $(cat "$work/csrf")" \
    -d "{\"values\":{\"title\":\"r$created\",\"n\":$created}}")
  t1=$(date +%s%N)
  if [ "$code" = 201 ]; then created=$((created + 1)); else errors=$((errors + 1)); fi
  ms=$(((t1 - t0) / 1000000))
  total_ms=$((total_ms + ms))
  samples=$((samples + 1))
  [ "$ms" -gt "$max_ms" ] && max_ms=$ms
  now=$(date +%s)
  if [ "$((now - last_sample))" -ge 5 ]; then
    last_sample=$now
    read -r cur_rss cur_fds cur_thr <<EOF
$(sample "$pid")
EOF
    [ "$cur_fds" -gt "$peak_fds" ] && peak_fds=$cur_fds
    [ "$cur_thr" -gt "$peak_thr" ] && peak_thr=$cur_thr
    [ "$cur_rss" -gt "$peak_rss" ] && peak_rss=$cur_rss
  fi
done

# Realtime subscriber: count SSE events for the last burst.
burst=20
sub=$(mktemp)
( curl -fsS -m 5 -N -b "$work/cj" "$base/api/v1/realtime" >"$sub" 2>/dev/null || true ) &
subpid=$!
for n in $(seq 1 "$burst"); do
  curl -fsS -o /dev/null -b "$work/cj" -X POST "$base/api/v1/collections/soak/records" \
    -H 'Content-Type: application/json' -H "Origin: $base" -H "X-Trestle-CSRF: $(cat "$work/csrf")" \
    -d "{\"values\":{\"title\":\"live$n\",\"n\":$n}}" || true
done
wait "$subpid" 2>/dev/null || true
events=$(grep -c '^data:' "$sub" || true)

# Files upload.
file_resp=$(curl -fsS -b "$work/cj" -X POST "$base/api/v1/files" \
  -H "X-Trestle-CSRF: $(cat "$work/csrf")" \
  -F "file=@$root/README.md")
file_id=$(printf '%s' "$file_resp" | sed -E 's/.*"id":"([^"]+)".*/\1/' | tr -d '\r\n')

# Backup.
backup=$(curl -fsS -b "$work/cj" -X POST "$base/admin/v1/backups" \
  -H 'Content-Type: application/json' -H "Origin: $base" -H "X-Trestle-CSRF: $(cat "$work/csrf")" -d '{}')
backup_id=$(printf '%s' "$backup" | sed -E 's/.*"id":"([^"]+)".*/\1/' | tr -d '\r\n')
curl -fsS -b "$work/cj" "$base/admin/v1/backups/$backup_id" -o "$work/soak-archive.zip"

# Let the runtime settle (GC) and sample several times, using the minimum
# RSS as the settled value so a single GC-timing point cannot fail the bound.
sleep 3
settled_rss=0; settled_fds=0; settled_thr=0
for i in 1 2 3; do
  sleep 1
  read -r cur_rss cur_fds cur_thr <<EOF
$(sample "$pid")
EOF
  if [ "$settled_rss" -eq 0 ] || [ "$cur_rss" -lt "$settled_rss" ]; then settled_rss=$cur_rss; fi
  [ "$cur_fds" -gt "$settled_fds" ] && settled_fds=$cur_fds
  [ "$cur_thr" -gt "$settled_thr" ] && settled_thr=$cur_thr
done
after="$settled_rss $settled_fds $settled_thr"
read -r after_rss after_fds after_thr <<EOF
$after
EOF
heap_after=$(heap_sample)

# Resource bounds against the SAME process (final settled vs warm baseline).
fd_growth=$((after_fds - $(echo "$before" | awk '{print $2}')))
thr_growth=$((after_thr - $(echo "$before" | awk '{print $3}')))
rss_growth=$((after_rss - $(echo "$before" | awk '{print $1}')))
if [ "$fd_growth" -gt 25 ]; then
  echo "fd growth $fd_growth exceeds bound 25" >&2; exit 1
fi
if [ "$thr_growth" -gt 20 ]; then
  echo "thread growth $thr_growth exceeds bound 20" >&2; exit 1
fi
# The RSS gate (50 MiB) is the primary bound. The gctrace heap values are
# reported as diagnostic evidence, not an authoritative leak signal: the
# before/after values may sit at different points in independent GC cycles.
# The duration series (30/60/120/300s) is consistent with bounded allocator
# retention (heap-live stayed 64/6/16/33 MB while records grew 828/1690/3280/
# 8369), but that is evidence consistent with bounded retention, not proof.
if [ "$rss_growth" -gt 51200 ]; then
  echo "settled RSS growth ${rss_growth} KiB exceeds bound 51200 KiB" >&2
  echo "diagnostics: baseline[$before] peak[$peak_rss $peak_fds $peak_thr] settled[$after] heap_before[$heap_before] heap_after[$heap_after] records=$created" >&2
  grep -oE 'gc [0-9]+ @[0-9.]+s .* MB' "$work/server.log" 2>/dev/null | tail -3 >&2
  exit 1
fi

# Webhook jobs: the 200-item newest-first list saturated with webhook jobs
# proves enqueue at scale; enqueue count and claim/drain/no-duplication are
# asserted by the provider-parameterized Go jobs suite, not by this list.
jobs_json=$(curl -fsS -b "$work/cj" "$base/admin/v1/jobs")
webhook_jobs=$(printf '%s' "$jobs_json" | grep -o '"kind":"webhook"' | wc -l)

# Assertions on the sustained phase: no API errors, realtime events delivered,
# backup produced, no secrets in logs, resources within bounds.
if [ "$errors" -gt 0 ]; then echo "CRUD errors: $errors" >&2; exit 1; fi
if [ "$created" -eq 0 ]; then echo "no records created" >&2; exit 1; fi
if [ "$events" -lt "$burst" ]; then echo "realtime events=$events, want >= $burst" >&2; exit 1; fi
if [ "$webhook_jobs" -lt 100 ]; then echo "webhook jobs in list=$webhook_jobs, want >= 100" >&2; exit 1; fi
if [ -z "$backup_id" ]; then echo "backup failed" >&2; exit 1; fi
if grep -q 'postgres://' "$work/server.log" 2>/dev/null; then
  echo "connection material leaked in logs" >&2; exit 1
fi

echo "soak sustained phase (this machine only):"
echo "  records created: $created"
echo "  api errors: 0"
echo "  realtime events observed: $events (burst $burst)"
echo "  webhook jobs in 200-cap list: $webhook_jobs (enqueue at scale)"
echo "  backup archive: $(stat -c %s "$work/soak-archive.zip") bytes"
echo "  baseline (VmRSS KB, fds, threads): $before"
echo "  peak    (VmRSS KB, fds, threads): $peak_rss $peak_fds $peak_thr"
echo "  final   (VmRSS KB, fds, threads): $after"
echo "  growth: fd ${fd_growth} (<=25) thread ${thr_growth} (<=20) settled RSS ${rss_growth} KiB (<=51200)"
echo "  heap (gctrace): before [$heap_before] after [$heap_after] (gc_count heap_live_MB elapsed_s)"
if [ "$samples" -gt 0 ]; then
  echo "  create latency avg: $((total_ms / samples)) ms  max: ${max_ms} ms  samples: $samples"
fi

# ---- Separate restart-recovery phase (not used in the leak comparison) ----
kill "$pid"; wait "$pid" 2>/dev/null || true; pid=""
"$work/trestle" --listen "127.0.0.1:$port" --data-dir "$work/data" >"$work/server2.log" 2>&1 &
pid=$!
i=0
until curl -fsS "$base/system/health" >/dev/null 2>&1; do
  i=$((i + 1)); [ "$i" -ge 80 ] && { cat "$work/server2.log" >&2; exit 1; }; sleep 0.1
done

# Verify recovery after restart: representative records present, file metadata
# present, backup visible, and the persisted admin session still authenticates.
record_id=$(curl -fsS -b "$work/cj" "$base/api/v1/collections/soak/records?limit=1" \
  | sed -E 's/.*"id":"([^"]+)".*/\1/' | tr -d '\r\n')
[ -n "$record_id" ] || { echo "no records after restart" >&2; exit 1; }
curl -fsS -b "$work/cj" "$base/api/v1/collections/soak/records/$record_id" >/dev/null || { echo "record not readable after restart" >&2; exit 1; }
[ -n "$file_id" ] && curl -fsS -b "$work/cj" "$base/api/v1/files/$file_id" >/dev/null || { echo "file metadata missing after restart" >&2; exit 1; }
curl -fsS -b "$work/cj" "$base/admin/v1/backups" | grep -q "$backup_id" || { echo "backup not visible after restart" >&2; exit 1; }
curl -fsS -b "$work/cj" "$base/admin/v1/collections" | grep -q '"soak"' || { echo "admin session invalid after restart" >&2; exit 1; }

echo "soak restart-recovery phase: records, file metadata, backup visibility and session behavior verified"
echo "soak passed"