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
# The required version input must carry NO default: the operator must enter the
# exact candidate tag deliberately. A default (e.g. a historical v0.1.0-rc.1)
# would make the rehearsal runnable without a deliberate choice.
if "default:" in re.sub(r"description:.*", "", text.split("workflow_dispatch")[1] if "workflow_dispatch" in text else text):
    issues.append("the rehearsal version input declares a default value")
if "default: v0.1.0-rc.1" in text:
    issues.append("the rehearsal version input still defaults to the historical v0.1.0-rc.1")
# Attestation policy flags: supported set required, --source-sha forbidden,
# signer-workflow is only the workflow path.
for flag in "--signer-workflow" "--source-ref" "--source-digest" "--deny-self-hosted-runners":
    if flag not in text:
        issues.append(f"attestation policy flag {flag} is missing")
if "--source-sha" in text:
    issues.append("forbidden --source-sha flag is present")
if "@refs/" in re.search(r"--signer-workflow[^\n]*", text).group(0) or "@" in text.split("--signer-workflow")[1].split("\n")[0]:
    issues.append("signer-workflow value contains an @ref suffix")
# The signer workflow is selected by the fail-closed legacy mapping: the
# workflow path is expressed through the VERIFY_REPO variable, the canonical
# identity is trestle-cv/trestle, and the known historical tags are explicitly
# mapped to the former trestle-dev/trestle identity rather than being accepted
# from a caller-controlled argument.
if "${VERIFY_REPO}/.github/workflows/release.yml" not in text:
    issues.append("signer-workflow does not use the mapped repository identity")
if "VERIFY_REPO=trestle-cv/trestle" not in text:
    issues.append("canonical identity is not the default")
if "VERIFY_REPO=trestle-dev/trestle" not in text or "v0.1.0-rc.1" not in text:
    issues.append("known historical tags are not mapped to the legacy identity")
# Tag-to-commit resolution and binary commit comparison.
if "commits/${version}" not in text or "expected_commit" not in text:
    issues.append("tag-to-commit resolution is missing")
if '${expected_commit}' not in text or "commit" not in text:
    issues.append("binary commit comparison is missing")
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
# First-run setup must supply the application registration policy the API
# requires; a stale payload would make the rehearsal fail on the fresh runner.
if "applicationRegistrationPolicy" not in text or '"closed"' not in text:
    issues.append("first-run setup does not provide the application registration policy")
# No secret/token printed.
for bad in 'echo "$pw"', 'echo "$GH_TOKEN"', 'cat cj', 'cat setup.json', 'curl -v':
    if bad in text:
        issues.append(f"workflow may print a secret/token: {bad}")
if "https://trestle.cv" not in text:
    issues.append("public script URLs are not used")
# Provenance reporting is diagnostic, NOT a second security policy: the
# authoritative gate is the constrained gh command; the jq requires only a
# non-empty array of verificationResult objects, prints the certificate
# verbatim (tojson), and must not depend on guessed certificate field names or
# label guessed identity fields.
if "validate_json" not in text or 'type == "array"' not in text:
    issues.append("non-empty verification-result array validation is missing")
if "tojson" not in text:
    issues.append("the certificate is not printed verbatim (tojson)")
if re.search(r"jq\s+-[a-z]+[^\n]*\|\s*jq", text) or re.search(r"jq -e[^\n]*\|\s*jq", text):
    issues.append("one jq is piped directly into another jq")
for label in '"repository: "', '"workflow: "', '"issuer: "', '"signerWorkflow: "', '"signerIssuer: "':
    if label in text:
        issues.append(f"reporting labels a guessed identity field: {label}")
for field in "sourceRepository", "SourceRepository", "subjectAlternativeName", "SubjectAlternativeName", 'issuer', 'Issuer':
    if field in text:
        issues.append(f"reporting depends on an unverified certificate field: {field}")
for i in issues:
    print("workflow issue:", i)
if issues:
    sys.exit(1)
print("release-rehearsal workflow is manual-only, read-only, injection-safe, RC-validated, tag-bound, policy-constrained attestation, diagnostic-only reporting, 7-char, signal-safe cleanup")
PY
[ "$?" -eq 0 ] || fail "release-rehearsal workflow structure"

# --- Executable behavioral regression for the provenance reporting jq --------
# Extract the exact validate/report programs from the merged
# "Verify build provenance and record details" step and drive them through
# valid and invalid attestation-result JSON. Certificate field presence or
# casing must NOT be treated as a security gate.
command -v jq >/dev/null 2>&1 || { echo "jq unavailable for the provenance regression" >&2; exit 1; }
vprog=$(mktemp "${TMPDIR:-/tmp}/trestle-rehearsal-validate.XXXXXX")
rprog=$(mktemp "${TMPDIR:-/tmp}/trestle-rehearsal-report.XXXXXX")
jqout=$(mktemp "${TMPDIR:-/tmp}/trestle-rehearsal-jq.XXXXXX")
trap 'rm -f "$vprog" "$rprog" "$jqout"' EXIT INT TERM
python3 - "$wf" "$vprog" "$rprog" <<'PY'
import sys, yaml
d = yaml.safe_load(open(sys.argv[1]))
steps = d["jobs"]["rehearse"]["steps"]
run = next(s["run"] for s in steps if s.get("name") == "Verify build provenance and record details")
def grab(name):
    start = run.index(name + "='") + len(name + "='")
    end = run.index("\n'", start)
    return run[start:end]
open(sys.argv[2], "w").write(grab("validate_json") + "\n")
open(sys.argv[3], "w").write(grab("report_json") + "\n")
PY
validate=$(cat "$vprog")
report=$(cat "$rprog")
[ -n "$validate" ] || fail "could not extract the validation jq program"
[ -n "$report" ] || fail "could not extract the reporting jq program"

validate_jq() { printf '%s' "$1" | jq -e "$validate" >/dev/null 2>&1; }
report_jq() { printf '%s' "$1" | jq -r "$report" >"$jqout" 2>/dev/null; }

# 1. A valid non-empty verificationResult array succeeds and the report emits
#    the subject names, the verbatim certificate and the timestamp count.
valid='[{"verificationResult":{"statement":{"subject":[{"name":"trestle_0.1.0-rc.1_linux_amd64.tar.gz"}]},"signature":{"certificate":{"SubjectAlternativeName":"https://github.com/trestle-dev/trestle/.github/workflows/release.yml@refs/tags/v0.1.0-rc.1"}},"verifiedTimestamps":[{"type":"RFC3161"}]}}]'
validate_jq "$valid" || fail "valid attestation array was rejected by the validation jq"
report_jq "$valid" || fail "valid attestation array was rejected by the reporting jq"
grep -q 'subjects: trestle_0.1.0-rc.1_linux_amd64.tar.gz' "$jqout" || fail "subjects not emitted"
grep -q 'certificate: {' "$jqout" || fail "certificate not emitted verbatim"
grep -q 'verifiedTimestamps: 1' "$jqout" || fail "verified timestamp count not emitted"

# 2. Certificate field presence/casing is NOT a security gate: a result with no
#    certificate still passes validation and the report prints what it can.
minimal='[{"verificationResult":{"statement":{"subject":[{"name":"asset"}]}}}]'
validate_jq "$minimal" || fail "validation treats missing certificate as a gate"
report_jq "$minimal" || fail "reporting failed on a result without a certificate"

# 3. Invalid forms fail validation: true, non-array object, empty array,
#    malformed JSON and an element missing verificationResult.
for bad in 'true' '{}' '[]' '{' '[{"foo":1}]'; do
  if validate_jq "$bad"; then fail "invalid attestation JSON accepted by the validation jq: $bad"; fi
done

if [ "$failures" -gt 0 ]; then
  echo "release-rehearsal regression: $failures failure(s)" >&2
  exit 1
fi
echo "release-rehearsal regression passed"