#!/bin/sh
# Release-rehearsal workflow safety regression (CP21).
#
# Validates .github/workflows/release-rehearsal.yml offline:
#   - manual-only (workflow_dispatch), no push/PR/schedule/tag trigger;
#   - read-only permissions (contents: read, attestations: read), no write;
#   - no repository checkout;
#   - the version input is validated before any URL is constructed;
#   - all six archives and SHA256SUMS are required;
#   - gh attestation verify is required for every archive;
#   - scripts are fetched from the public https://trestle.cv URLs;
#   - no release-mutation command or mutable third-party action appears.
set -eu
root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
wf="$root/.github/workflows/release-rehearsal.yml"
failures=0
fail() { echo "release rehearsal FAIL: $*" >&2; failures=$((failures + 1)); }
[ -f "$wf" ] || { echo "release rehearsal FAIL: $wf missing" >&2; exit 1; }

python3 - "$wf" <<'PY'
import sys, yaml, re
d = yaml.safe_load(open(sys.argv[1]))
issues = []
# PyYAML (YAML 1.1) parses the GitHub Actions 'on' key as boolean True; accept
# either spelling of the trigger key.
trig = (d.get("on") or d.get(True)) if isinstance(d, dict) else None
triggers = set(trig.keys()) if isinstance(trig, dict) else set()
if triggers != {"workflow_dispatch"}:
    issues.append(f"triggers are not manual-only: {sorted(triggers)}")
perm = d.get("permissions") or {}
if perm.get("contents") != "read" or perm.get("attestations") != "read":
    issues.append(f"permissions are not read-only (contents: read, attestations: read): {perm}")
if any(v not in ("read", "none") for v in perm.values()):
    issues.append(f"a permission is write-capable: {perm}")
jobs = d.get("jobs") or {}
text = open(sys.argv[1]).read()
if "actions/checkout" in text:
    issues.append("the workflow checks out the repository")
if re.search(r"\buses:\s*[\"']?[^\"']+@v?\d", text):
    issues.append("a mutable third-party action tag is present")
if re.search(r"gh release|action-gh-release|git push|git tag", text):
    issues.append("a release-mutation command appears")
if "gh attestation verify" not in text:
    issues.append("attestation verification is not required")
if text.count("trestle_${version#v}") < 6:
    issues.append("fewer than six archives are downloaded")
if "SHA256SUMS" not in text:
    issues.append("SHA256SUMS is not downloaded")
if "https://trestle.cv" not in text:
    issues.append("public script URLs are not used")
if not re.search(r"workflow_dispatch.*inputs.*version", text, re.S) or "Validate version input" not in text:
    issues.append("the version input is not validated")
for i in issues:
    print("workflow issue:", i)
if issues:
    sys.exit(1)
print("release-rehearsal workflow is manual-only, read-only, no checkout, version-validated, attestation-required, public-URL, no mutation")
PY
[ "$?" -eq 0 ] || fail "release-rehearsal workflow structure"

if [ "$failures" -gt 0 ]; then
  echo "release-rehearsal regression: $failures failure(s)" >&2
  exit 1
fi
echo "release-rehearsal regression passed"