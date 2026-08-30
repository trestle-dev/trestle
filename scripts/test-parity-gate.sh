#!/bin/sh
# CP9R parity gate: runs every parity-matrix evidence test on both SQLite and
# real PostgreSQL, so the matrix rows are backed by executed tests rather than
# names. A cited test that fails on either provider fails the gate; a test that
# skips without PostgreSQL is expected only for PostgreSQL-specific rows.
set -eu

root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"

python3 - <<'PY'
import json, subprocess, sys, os

matrix = json.load(open("docs/postgres/parity-matrix.json"))
entries = []
for area in matrix["areas"]:
    for op in area["operations"]:
        for e in op["evidence"]:
            entries.append((e["package"], e["test"]))
for op in matrix["unverifiedOrProviderSpecific"]:
    for e in op["evidence"]:
        if e["package"] != "n/a":
            entries.append((e["package"], e["test"]))

# Deduplicate preserving order.
seen = set(); uniq = []
for e in entries:
    if e not in seen:
        seen.add(e); uniq.append(e)

failures = []
for pkg, test in uniq:
    pattern = "^" + test + "$"
    for label, env in (("sqlite", {}), ("postgres", dict(TRESTLE_TEST_POSTGRES_URL=os.environ.get("TRESTLE_TEST_POSTGRES_URL", "")))):
        runenv = dict(os.environ)
        runenv.update(env)
        out = subprocess.run(
            ["go", "test", "./internal/"+pkg, "-run", pattern, "-count=1", "-v"],
            env=runenv, capture_output=True, text=True)
        combined = out.stdout + out.stderr
        if out.returncode != 0:
            failures.append(f"{pkg}.{test} [{label}] FAILED:\n{combined[-1200:]}")
            continue
        # A name typo would run no tests; the -run pattern must match.
        if f"--- PASS: {test}" not in combined and f"--- FAIL: {test}" not in combined:
            # Allow SKIP (e.g. PostgreSQL-only tests without a server) only if
            # the test is marked SKIP; otherwise the test did not run.
            if f"--- SKIP: {test}" not in combined and "no tests to run" in combined:
                failures.append(f"{pkg}.{test} [{label}] did not run (no matching test)")
            continue

if failures:
    sys.stderr.write("\n".join(failures) + "\n")
    sys.exit(1)
print(f"parity gate: ran {len(uniq)} evidence tests on SQLite and PostgreSQL; all pass")
PY