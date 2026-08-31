#!/bin/sh
# Release-rehearsal workflow safety regression (CP21).
#
# Validates .github/workflows/release-rehearsal.yml offline:
#   - manual-only, read-only permissions, no checkout, no release mutation;
#   - no workflow input expression inside any run: shell source (input enters
#     only through the step environment);
#   - the RC-format validator rejects hostile strings (quotes, command
#     substitution, semicolons, control characters) and accepts valid RCs;
#   - tag->commit resolution and binary commit comparison are present;
#   - attestation uses the supported CLI flags --signer-workflow,
#     --source-ref, --source-digest and --deny-self-hosted-runners; --source-sha
#     is forbidden; the signer-workflow value is only the workflow path;
#   - JSON provenance output is validated as an array;
#   - INT/TERM traps clean up and then exit; cleanup always waits for the
#     service;
#   - no password, cookie, CSRF or token value is printed.
set -eu
root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
wf="$root/.github/workflows/release-rehearsal.yml"
failures=0
fail() { echo "release rehearsal FAIL: $*" >&2; failures=$((failures + 1)); }
[ -f "$wf" ] || { echo "release rehearsal FAIL: $wf missing" >&2; exit 1; }

# --- Executable RC-version validation using the workflow's own pattern. ------
pattern=$(sed -n "s/.*grep -q '\(\^v[^']*\)'.*/\1/p" "$wf" | head -1)
[ -n "$pattern" ] || fail "could not extract the version validation pattern"
nonprintable() {
  # grep is line-based and cannot see newlines; detect them via wc -l too.
  [ "$(printf '%s' "$1" | wc -l)" -gt 0 ] || printf '%s' "$1" | LC_ALL=C grep -q '[^ -~]' 2>/dev/null
}
accept() { printf '%s\n' "$1" | LC_ALL=C grep -q "$pattern" 2>/dev/null; }
reject() { nonprintable "$1" || ! accept "$1"; }
for v in v0.1.0-rc.1 v0.1.0-rc.42 v1.2.3-rc.10 v0.0.0-rc.9; do
  accept "$v" || fail "valid RC '$v' rejected"
done
# Hostile / malformed strings at the validator boundary.
for v in \
  v0.1.0 v0.1.0-rc v0.1.0-rc.0 v0.1.0-rc.01 v0.1.0-rc.1. v0.1.0-rc.1..2 \
  v0.1.0-rc.1+build v0.1.0-beta.1 v0.1.0-rc.1-rc.2 "v0.1.0-rc.1 " \
  'v0.1.0-rc.1"; touch /tmp/pwned; "' \
  'v0.1.0-rc.1; echo pwned' \
  'v0.1.0-rc.1$(whoami)' \
  "v0.1.0-rc.1'" \
  "$(printf 'v0.1.0-rc.1\n; pwned')" \
  "$(printf 'v0.1.0-rc.1\r\n')"; do
  reject "$v" || fail "invalid/hostile version accepted: $(printf '%s' "$v" | od -c | head -1)"
done

python3 - "$wf" <<'PY'
import sys, yaml, re
d = yaml.safe_load(open(sys.argv[1]))
text = open(sys.argv[1]).read()
issues = []
trig = (d.get("on") or d.get(True)) if isinstance(d, dict) else None
triggers = set(trig.keys()) if isinstance(trig, dict) else set()
if triggers != {"workflow_dispatch"}:
    issues.append(f"triggers are not manual-only: {sorted(triggers)}")
perm = d.get("permissions") or {}
if perm.get("contents") != "read" or perm.get("attestations") != "read":
    issues.append(f"permissions are not read-only: {perm}")
if any(v not in ("read", "none") for v in perm.values()):
    issues.append(f"a permission is write-capable: {perm}")
if "actions/checkout" in text:
    issues.append("the workflow checks out the repository")
if re.search(r"\buses:\s*[\"']?[^\"']+@v?\d", text):
    issues.append("a mutable third-party action tag is present")
if re.search(r"gh release|action-gh-release|git push|git tag", text):
    issues.append("a release-mutation command appears")
# No workflow input expression inside any run: shell source.
for job, jd in (d.get("jobs") or {}).items():
    for s in jd.get("steps", []):
        if isinstance(s, dict) and "run" in s and "${{" in s["run"]:
            issues.append(f"workflow input expression inside a run: body in job {job}")
# Version enters through the step environment.
if "INPUT_VERSION: ${{ inputs.version }}" not in text and "INPUT_VERSION: ${{ github.event.inputs.version }}" not in text:
    issues.append("version input is not passed through the step environment")
if "version=$INPUT_VERSION" not in text:
    issues.append("version is not read from the environment variable")
# Attestation policy flags: supported set required, --source-sha forbidden,
# signer-workflow is only the workflow path.
for flag in "--signer-workflow" "--source-ref" "--source-digest" "--deny-self-hosted-runners":
    if flag not in text:
        issues.append(f"attestation policy flag {flag} is missing")
if "--source-sha" in text:
    issues.append("forbidden --source-sha flag is present")
if "@refs/" in re.search(r"--signer-workflow[^\n]*", text).group(0) or "@" in text.split("--signer-workflow")[1].split("\n")[0]:
    issues.append("signer-workflow value contains an @ref suffix")
if "trestle-dev/trestle/.github/workflows/release.yml" not in text:
    issues.append("signer-workflow does not name the release workflow path")
# Tag-to-commit resolution and binary commit comparison.
if "commits/${version}" not in text or "expected_commit" not in text:
    issues.append("tag-to-commit resolution is missing")
if '${expected_commit}' not in text or "commit" not in text:
    issues.append("binary commit comparison is missing")
# JSON provenance output validated as an array.
if "--format json" in text and 'type == "array"' not in text:
    issues.append("structured JSON output is not validated as an array")
# INT/TERM traps exit after cleanup; cleanup waits for the service.
if "trap 'cleanup; exit 130' INT" not in text or "trap 'cleanup; exit 143' TERM" not in text:
    issues.append("INT/TERM traps do not clean up and exit")
if "cleanup()" not in text or 'wait "$svc"' not in text:
    issues.append("cleanup does not wait for the service")
if "trap cleanup EXIT" not in text:
    issues.append("cleanup is not bound to EXIT")
# Exactly-seven-character password acceptance exercised.
if 'pw="1234567"' not in text:
    issues.append("exactly-seven-character password acceptance is not exercised")
# No secret/token printed.
for bad in 'echo "$pw"', 'echo "$GH_TOKEN"', 'cat cj', 'cat setup.json', 'curl -v':
    if bad in text:
        issues.append(f"workflow may print a secret/token: {bad}")
if "https://trestle.cv" not in text:
    issues.append("public script URLs are not used")
for i in issues:
    print("workflow issue:", i)
if issues:
    sys.exit(1)
print("release-rehearsal workflow is manual-only, read-only, injection-safe, RC-validated, tag-bound, policy-constrained attestation, array-validated JSON, 7-char, signal-safe cleanup")
PY
[ "$?" -eq 0 ] || fail "release-rehearsal workflow structure"

if [ "$failures" -gt 0 ]; then
  echo "release-rehearsal regression: $failures failure(s)" >&2
  exit 1
fi
echo "release-rehearsal regression passed"