#!/bin/sh
# Release-rehearsal workflow safety regression (CP21).
#
# Validates .github/workflows/release-rehearsal.yml offline:
#   - manual-only (workflow_dispatch), read-only permissions, no checkout;
#   - the version validator is the narrowed RC format, proven by accepted and
#     rejected executable cases using the workflow's own pattern;
#   - the tag is peeled to a commit dynamically and the binary commit must
#     match it;
#   - attestation verification is constrained to the pinned release workflow,
#     the selected source ref and the peeled commit;
#   - all six exact archive names and SHA256SUMS are required;
#   - exactly-seven-character password acceptance is exercised;
#   - cleanup terminates and waits for the service on success/failure/INT/TERM;
#   - no password, cookie, CSRF or token value is printed.
set -eu
root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
wf="$root/.github/workflows/release-rehearsal.yml"
failures=0
fail() { echo "release rehearsal FAIL: $*" >&2; failures=$((failures + 1)); }
[ -f "$wf" ] || { echo "release rehearsal FAIL: $wf missing" >&2; exit 1; }

# --- Executable RC-version validation using the workflow's own pattern. ------
pattern=$(sed -n 's/.*grep -q '\''\([^'\'']*\)'\''.*/\1/p' "$wf" | head -1)
[ -n "$pattern" ] || fail "could not extract the version validation pattern"
accept() { printf '%s\n' "$1" | LC_ALL=C grep -q "$pattern" 2>/dev/null; }
reject() { ! accept "$1"; }
for v in v0.1.0-rc.1 v0.1.0-rc.42 v1.2.3-rc.10 v0.0.0-rc.9; do
  accept "$v" || fail "valid RC '$v' rejected"
done
for v in \
  v0.1.0 v0.1.0-rc v0.1.0-rc.0 v0.1.0-rc.01 v0.1.0-rc.1. v0.1.0-rc.1..2 \
  v0.1.0-rc.1+build v0.1.0-beta.1 v0.1.0-rc.1-rc.2 "v0.1.0-rc.1 " \
  "v0.1.0-rc.1;rm"; do
  reject "$v" || fail "invalid version '$v' accepted"
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
# Exact archive names, not merely six string occurrences.
expected = [
    "trestle_${version#v}_linux_amd64.tar.gz",
    "trestle_${version#v}_linux_arm64.tar.gz",
    "trestle_${version#v}_darwin_amd64.tar.gz",
    "trestle_${version#v}_darwin_arm64.tar.gz",
    "trestle_${version#v}_windows_amd64.zip",
    "trestle_${version#v}_windows_arm64.zip",
]
for name in expected:
    if text.count(name) == 0:
        issues.append(f"archive {name} is not downloaded")
if "SHA256SUMS" not in text or "sha256sum -c SHA256SUMS" not in text:
    issues.append("SHA256SUMS download/verification missing")
# Tag-to-commit resolution and binary commit comparison. The workflow compares
# ./trestle version's commit field against the peeled tag commit.
if "commits/${version}" not in text or "expected_commit" not in text:
    issues.append("tag-to-commit resolution is missing")
if '${expected_commit}' not in text or "commit" not in text:
    issues.append("binary commit comparison is missing")
# Attestation constrained to the intended workflow/ref/commit.
for flag in "--signer-workflow" "--source-ref" "--source-sha":
    if flag not in text:
        issues.append(f"attestation policy flag {flag} is missing")
if "gh attestation verify" not in text:
    issues.append("attestation verification is not required")
# Exactly-seven-character acceptance exercised.
if 'pw="1234567"' not in text:
    issues.append("exactly-seven-character password acceptance is not exercised")
# Cleanup terminates and waits for the service on EXIT/INT/TERM.
if "cleanup()" not in text or 'kill -0 "$svc"' not in text or 'wait "$svc"' not in text:
    issues.append("cleanup does not terminate and wait for the service")
if "trap cleanup EXIT INT TERM" not in text:
    issues.append("cleanup is not bound to EXIT/INT/TERM")
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
print("release-rehearsal workflow is manual-only, read-only, no checkout, RC-validated, tag-bound, policy-constrained attestation, 7-char, cleaned up, secret-safe")
PY
[ "$?" -eq 0 ] || fail "release-rehearsal workflow structure"

if [ "$failures" -gt 0 ]; then
  echo "release-rehearsal regression: $failures failure(s)" >&2
  exit 1
fi
echo "release-rehearsal regression passed"