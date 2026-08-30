#!/bin/sh
# Release-contract regression: the release workflow, packaging and release
# notes must satisfy the invariants the public scripts and the human runbook
# rely on. Runs entirely offline; nothing is tagged, built or published.
set -eu
root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
failures=0
fail() { echo "release contract FAIL: $*" >&2; failures=$((failures + 1)); }

# 1. Workflow YAML is valid and carries the required release wiring.
python3 - "$root" <<'PY'
import json, sys, yaml, re
root = sys.argv[1]
d = yaml.safe_load(open(root + "/.github/workflows/release.yml"))
jobs = d["jobs"]
checks = []

# Every action in the privileged release workflow must be pinned to a full
# 40-hex commit SHA (with an optional human-readable version comment), never a
# mutable major-version tag.
pinned = {
    "actions/checkout": "3d3c42e5aac5ba805825da76410c181273ba90b1",
    "actions/setup-go": "b7ad1dad31e06c5925ef5d2fc7ad053ef454303e",
    "actions/upload-artifact": "043fb46d1a93c77aae656e7c1c64a875d1fc6a0a",
    "actions/download-artifact": "3e5f45b2cfb9172054b4087a40e8e0b5a5461e7c",
    "actions/attest-build-provenance": "4d101475d8b20a2381f78447822ac1eab6504dd8",
    "softprops/action-gh-release": "efb35369e0ad2afab669f228072c1b0d510eae64",
}
uses = []
for job, jd in jobs.items():
    for s in jd.get("steps", []):
        if isinstance(s, dict) and "uses" in s:
            uses.append(s["uses"])
if not uses:
    checks.append("release workflow has no action uses")
for u in uses:
    m = re.match(r"^([A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+)@([0-9a-f]{40})(\s+# .+)?$", u)
    if not m:
        checks.append(f"release workflow action is not pinned to a full commit SHA: {u}")
        continue
    name, sha = m.group(1), m.group(2)
    if name in pinned and sha != pinned[name]:
        checks.append(f"release workflow action {name} pinned to an unexpected SHA {sha}")
if set(pinned) - {u.split("@")[0] for u in uses}:
    missing = set(pinned) - {u.split("@")[0] for u in uses}
    checks.append(f"expected pinned action absent: {sorted(missing)}")

# Privilege separation: build is read-only, attest holds exactly the
# documented provenance contract (contents: read, id-token: write,
# attestations: write) with no extra write permission, and only publish holds
# contents: write.
if d.get("permissions", {}).get("contents") != "read":
    checks.append("workflow-level permissions are not contents: read")
perm = lambda j: jobs[j].get("permissions", {})
if "build" not in jobs or any(v != "none" and v != "read" for v in perm("build").values() if v):
    checks.append("build job is not read-only")
expected_attest = {"contents": "read", "id-token": "write", "attestations": "write"}
if perm("attest") != expected_attest:
    checks.append(f"attestation job permissions are not exactly {sorted(expected_attest)}: {dict(perm('attest'))}")
for scope, value in perm("attest").items():
    if scope not in ("contents", "id-token", "attestations") and value not in ("none", "read"):
        checks.append(f"attestation job carries an unexpected write permission: {scope}={value}")
if perm("publish").get("contents") != "write":
    checks.append("publish job does not hold contents: write")

steps = jobs["build"]["steps"]
names = [s.get("name") or next(iter(s)) for s in steps]
body_path_ok = any("body_path" in s.get("with", {}) for s in jobs["publish"]["steps"])
gen_notes_ok = any(s.get("with", {}).get("generate_release_notes") is True for s in jobs["publish"]["steps"])
package_ok = any("package-release.sh" in (s.get("run") or "") for s in steps)
files_ok = any("dist/*" in (s.get("with", {}).get("files") or "") for s in jobs["publish"]["steps"])
subject_ok = any("dist/*" in (s.get("with", {}).get("subject-path") or "") for s in jobs["attest"]["steps"])
print("jobs:", list(jobs))
print("build steps:", names)
if not body_path_ok: checks.append("publish step has no body_path (release-notes body)")
if not gen_notes_ok: checks.append("publish step has no generate_release_notes")
if not package_ok: checks.append("build job does not run package-release.sh")
if not files_ok: checks.append("publish step does not upload dist/*")
if not subject_ok: checks.append("attest step does not attest dist/*")
for c in checks: print("workflow issue:", c)
if checks: sys.exit(1)
PY
[ "$?" -eq 0 ] || fail "release workflow wiring"

# 2. The release-notes body satisfies the operational requirements after
#    version substitution.
body=$(sed "s/{{VERSION}}/0.1.0/" "$root/docs/release-notes-template.md")
[ -n "$body" ] || fail "release-notes body is empty"
case "$body" in
  *'{{VERSION}}'*) fail "unresolved {{VERSION}} placeholder remains in the release body" ;;
esac
echo "$body" | grep -q "0.1.0" || fail "expected version is absent from the release body"
for heading in "preview / release candidate" "PostgreSQL" "SQLite" "Operator responsibilities" "Back up before upgrading" "Known limitations" "Verified installation"; do
  echo "$body" | grep -qi "$heading" || fail "release body lacks required statement: $heading"
done

# 3. Asset contract: the six packaged targets match the public scripts'
#    os/arch naming, and package-release.sh emits SHA256SUMS with the exact
#    two-space format the scripts' parser requires.
grep -q 'for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64' "$root/scripts/package-release.sh" \
  || fail "package-release.sh target list does not match the six-platform contract"
grep -q 'sha256sum trestle_\* > SHA256SUMS' "$root/scripts/package-release.sh" \
  || fail "package-release.sh does not emit SHA256SUMS"
grep -q 'SOURCE_DATE_EPOCH' "$root/scripts/package-release.sh" \
  || fail "package-release.sh is not reproducible (no SOURCE_DATE_EPOCH/commit-derived date)"

# 4. The three public scripts agree on version/archive naming and the
#    SHA256SUMS URL, and each requires verification before extraction.
for script in install download update; do
  f="$root/scripts/public/$script.sh"
  grep -q 'archive="trestle_\${version#v}_\${os}_\${arch}.tar.gz"' "$f" \
    || fail "$script.sh asset naming does not match the release contract"
  grep -q 'SHA256SUMS' "$f" || fail "$script.sh does not fetch SHA256SUMS"
  grep -q 'verify_archive' "$f" || fail "$script.sh does not verify before extraction"
done

if [ "$failures" -gt 0 ]; then
  echo "release-contract regression: $failures failure(s)" >&2
  exit 1
fi
echo "release-contract regression passed: workflow wiring, release-notes body, asset contract, script agreement"