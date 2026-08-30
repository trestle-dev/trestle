#!/bin/sh
# CP9R2 parity gate: runs every parity-matrix evidence test on SQLite and real
# PostgreSQL with truly isolated environments, records PASS/SKIP/FAIL per test
# per provider, and fails a matrix row that claims a provider is verified when
# no cited test actually passes on that provider.
set -eu

root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"

python3 - <<'PY'
import json, os, subprocess, sys

matrix = json.load(open("docs/postgres/parity-matrix.json"))

# Collect rows with their claimed provider status and evidence.
rows = []
for area in matrix["areas"]:
    for op in area["operations"]:
        rows.append({"operation": op["operation"], "sqlite": op["sqlite"],
                     "postgres": op["postgres"], "evidence": op["evidence"]})
for op in matrix["unverifiedOrProviderSpecific"]:
    rows.append({"operation": op["operation"], "sqlite": op["sqlite"],
                 "postgres": op["postgres"], "evidence": op["evidence"]})

# Unique (package, test) entries.
entries = []
seen = set()
for row in rows:
    for e in row["evidence"]:
        key = (e["package"], e["test"])
        if e["package"] != "n/a" and key not in seen:
            seen.add(key)
            entries.append({"package": e["package"], "test": e["test"],
                            "rows": [row for row in rows if e in row["evidence"]]})

failures = []
results = {}  # (package,test) -> {"sqlite": status, "postgres": status}
for entry in entries:
    pkg, test = entry["package"], entry["test"]
    pattern = "^" + test + "$"
    for provider in ("sqlite", "postgres"):
        runenv = dict(os.environ)
        if provider == "sqlite":
            # Truly isolate the SQLite leg: the PostgreSQL URL must not leak in.
            runenv.pop("TRESTLE_TEST_POSTGRES_URL", None)
        else:
            if "TRESTLE_TEST_POSTGRES_URL" not in runenv or not runenv["TRESTLE_TEST_POSTGRES_URL"]:
                failures.append(f"{pkg}.{test} [{provider}]: TRESTLE_TEST_POSTGRES_URL is required")
                results.setdefault((pkg, test), {})[provider] = "FAIL"
                continue
        out = subprocess.run(
            ["go", "test", "./internal/"+pkg, "-run", pattern, "-count=1", "-v"],
            env=runenv, capture_output=True, text=True)
        combined = out.stdout + out.stderr
        status = "FAIL"
        if out.returncode != 0:
            status = "FAIL"
        elif f"--- PASS: {test}" in combined:
            status = "PASS"
        elif f"--- SKIP: {test}" in combined or "SKIP:" in combined:
            status = "SKIP"
        elif "no tests to run" in combined:
            failures.append(f"{pkg}.{test} [{provider}]: did not run (no matching test)")
            status = "FAIL"
        results.setdefault((pkg, test), {})[provider] = status

# Fail a matrix row that claims a provider is verified but has no passing test
# on that provider.
for row in rows:
    if row["sqlite"] == "verified":
        if not any(results.get((e["package"], e["test"]), {}).get("sqlite") == "PASS" for e in row["evidence"]):
            failures.append(f"row '{row['operation']}' claims sqlite=verified but no cited test PASSES on sqlite")
    if row["postgres"] == "verified":
        if not any(results.get((e["package"], e["test"]), {}).get("postgres") == "PASS" for e in row["evidence"]):
            failures.append(f"row '{row['operation']}' claims postgres=verified but no cited test PASSES on postgres")

if failures:
    sys.stderr.write("\n".join(failures) + "\n")
    sys.exit(1)

print("parity gate: per-test provider execution (sqlite with PG URL removed, postgres with PG URL required):")
for (pkg, test), prov in sorted(results.items()):
    print(f"  {pkg}.{test}: sqlite={prov.get('sqlite','-')} postgres={prov.get('postgres','-')}")
print(f"parity gate: {len(entries)} evidence tests; all rows marked verified are backed by a passing test on the claimed provider")
PY