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

results = []
failures = []
for pkg, test in entries:
    pattern = "^" + test + "$"
    out = subprocess.run(["go", "test", "./internal/"+pkg, "-run", pattern, "-count=1", "-v"],
                         env=os.environ, capture_output=True, text=True)
    combined = out.stdout + out.stderr
    if "--- PASS: " + test in combined:
        results.append((f"{pkg}.{test}", "PASS"))
    elif "--- SKIP: " + test in combined:
        # A proven surface's cited evidence must actually execute; a skip is
        # reported explicitly and treated as not proven for this run.
        results.append((f"{pkg}.{test}", "SKIP"))
        failures.append(f"{pkg}.{test} SKIPPED (cited as proven evidence but did not execute)")
    elif out.returncode != 0:
        results.append((f"{pkg}.{test}", "FAIL"))
        failures.append(f"{pkg}.{test} FAILED:\n{combined[-1000:]}")
    elif "no tests to run" in combined:
        results.append((f"{pkg}.{test}", "NONE"))
        failures.append(f"{pkg}.{test} did not run (no matching test)")
    else:
        results.append((f"{pkg}.{test}", "FAIL"))
        failures.append(f"{pkg}.{test} did not report PASS or SKIP")

for name, status in results:
    print(f"  {name}: {status}")
if failures:
    sys.stderr.write("\n".join(failures) + "\n")
    sys.exit(1)
passed = sum(1 for _, s in results if s == "PASS")
print(f"subsystem gate: {passed} of {len(entries)} cited proven tests PASS (none skipped or failed)")
PY
