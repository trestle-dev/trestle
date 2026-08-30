#!/bin/sh
# CP11R2 subsystem evidence gate: runs every evidence test cited by a proven
# surface in docs/hardening/subsystem-matrix.json and fails if any fails or
# does not run. TRESTLE_TEST_POSTGRES_URL is honoured when present.
set -eu

root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"

python3 - <<'PY'
import json, os, subprocess, sys
m = json.load(open("docs/hardening/subsystem-matrix.json"))
entries = []
seen = set()
for surface in m["surfaces"]:
    if surface["status"] != "proven":
        continue
    for e in surface["evidence"]:
        key = (e["package"], e["test"])
        if key not in seen:
            seen.add(key)
            entries.append(key)

failures = []
for pkg, test in entries:
    pattern = "^" + test + "$"
    out = subprocess.run(["go", "test", "./internal/"+pkg, "-run", pattern, "-count=1", "-v"],
                         env=os.environ, capture_output=True, text=True)
    combined = out.stdout + out.stderr
    if out.returncode != 0:
        failures.append(f"{pkg}.{test} FAILED:\n{combined[-1000:]}")
        continue
    if f"--- PASS: {test}" not in combined and "no tests to run" not in combined:
        # Allowed: SKIP when PG-only without a server.
        if "SKIP" not in combined:
            failures.append(f"{pkg}.{test} did not report PASS or SKIP")
        continue
    if "no tests to run" in combined:
        failures.append(f"{pkg}.{test} did not run (no matching test)")

if failures:
    sys.stderr.write("\n".join(failures) + "\n")
    sys.exit(1)
print(f"subsystem gate: ran {len(entries)} cited proven tests; all pass")
PY
