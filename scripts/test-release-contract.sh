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
import json, sys, yaml
root = sys.argv[1]
d = yaml.safe_load(open(root + "/.github/workflows/release.yml"))
steps = d["jobs"]["release"]["steps"]
names = [s.get("name") or next(iter(s)) for s in steps]
checks = []
body_path_ok = any("body_path" in s.get("with", {}) for s in steps)
gen_notes_ok = any(s.get("with", {}).get("generate_release_notes") is True for s in steps)
package_ok = any("package-release.sh" in (s.get("run") or "") for s in steps)
files_ok = any("dist/*" in (s.get("with", {}).get("files") or "") for s in steps)
subject_ok = any("dist/*" in (s.get("with", {}).get("subject-path") or "") for s in steps)
print("steps:", names)
if not body_path_ok: checks.append("release workflow has no body_path (release-notes body)")
if not gen_notes_ok: checks.append("release workflow has no generate_release_notes")
if not package_ok: checks.append("release workflow does not run package-release.sh")
if not files_ok: checks.append("release workflow does not upload dist/*")
if not subject_ok: checks.append("release workflow does not attest dist/*")
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