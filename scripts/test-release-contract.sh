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
#    version substitution, and the maturity wording matches the release kind:
#    stable versions never get release-candidate wording; prerelease versions
#    keep honest prerelease wording; unresolved or contradictory wording fails.
release_kind() { # release_kind VERSION prints kind then body on two lines (matches the workflow)
  case "${1#v}" in
    *-*) printf 'preview release candidate\nis a preview / release candidate, not a stable release. Use it to test an upcoming release; prereleases must be selected explicitly by version.\n' ;;
    *) printf 'stable public preview\nestablishes the stable download channel and the initial public compatibility contract. It is ready for public preview and evaluation, but is not yet claimed to be production-proven or battle-proven.\n' ;;
  esac
}
build_body() { # build_body VERSION applies the workflow's substitution
  kind=$(release_kind "$1" | sed -n '1p')
  body=$(release_kind "$1" | sed -n '2p')
  sed -e "s|{{VERSION}}|${1#v}|" \
      -e "s|{{RELEASE_KIND}}|${kind}|" \
      -e "s|{{RELEASE_KIND_BODY}}|${body}|" \
      "$root/docs/release-notes-template.md"
}
stable_body=$(build_body v0.1.0)
prerelease_body=$(build_body v0.1.0-rc.1)
[ -n "$stable_body" ] && [ -n "$prerelease_body" ] || fail "release-notes body is empty"
for placeholder in '{{VERSION}}' '{{RELEASE_KIND}}' '{{RELEASE_KIND_BODY}}'; do
  case "$stable_body" in *"$placeholder"*) fail "unresolved placeholder $placeholder in the stable body" ;; esac
  case "$prerelease_body" in *"$placeholder"*) fail "unresolved placeholder $placeholder in the prerelease body" ;; esac
done
echo "$stable_body" | grep -q "0.1.0" || fail "expected stable version is absent"
echo "$prerelease_body" | grep -q "0.1.0-rc.1" || fail "expected prerelease version is absent"
# Stable: must lead with stable-public-preview wording and never call itself a
# release candidate or "not a stable release".
echo "$stable_body" | grep -qi "stable public preview" || fail "stable body lacks stable-public-preview wording"
echo "$stable_body" | grep -qi "release candidate" && fail "stable body calls itself a release candidate"
echo "$stable_body" | grep -qi "not a stable release" && fail "stable body says it is not a stable release"
# Prerelease: must honestly say it is a preview release candidate.
echo "$prerelease_body" | grep -qi "preview / release candidate" || fail "prerelease body lacks release-candidate wording"
echo "$prerelease_body" | grep -qi "not a stable release" || fail "prerelease body must say it is not a stable release"
# Operational headings present in both.
for heading in "Supported database options" "Operator responsibilities" "Back up before upgrading" "Known limitations" "Verified installation"; do
  echo "$stable_body" | grep -qi "$heading" || fail "stable body lacks required statement: $heading"
  echo "$prerelease_body" | grep -qi "$heading" || fail "prerelease body lacks required statement: $heading"
done
# Contradictory maturity wording must fail validation: a stable body that also
# contains release-candidate wording is invalid.
if echo "$stable_body" | grep -qi "release candidate"; then
  fail "contradictory maturity wording was not rejected"
fi

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

# 5. Prerelease tag semantics (CP21): a tag with a semver prerelease suffix
#    must publish a GitHub prerelease so /releases/latest never selects it.
#    The workflow must compute this explicitly (the release action does not
#    infer it), and the offline test evaluates the same detection rule.
detect_prerelease() {
  case "${1#v}" in *-*) echo true ;; *) echo false ;; esac
}
[ "$(detect_prerelease v0.1.0)" = false ] || fail "stable tag v0.1.0 misdetected as prerelease"
[ "$(detect_prerelease v0.1.0-rc.1)" = true ] || fail "prerelease tag v0.1.0-rc.1 misdetected as stable"
[ "$(detect_prerelease v1.2.3-beta.2)" = true ] || fail "beta tag misdetected as stable"
[ "$(detect_prerelease v1.2.3+build.5)" = false ] || fail "build-metadata tag misdetected as prerelease"
python3 - "$root" <<'PY'
import sys, yaml
root = sys.argv[1]
d = yaml.safe_load(open(root + "/.github/workflows/release.yml"))
pub = d["jobs"]["publish"]["steps"]
has_detection = any("is-prerelease" in (s.get("run") or "") and "GITHUB_REF_NAME" in (s.get("run") or "") for s in pub)
rel = next((s for s in pub if isinstance(s, dict) and "uses" in s and "action-gh-release" in s["uses"]), {})
prerelease_input = (rel.get("with") or {}).get("prerelease")
if not has_detection:
    print("workflow issue: publish job lacks explicit prerelease detection")
    sys.exit(1)
if prerelease_input != "${{ steps.prerelease.outputs.is-prerelease }}":
    print(f"workflow issue: release action prerelease input is {prerelease_input!r}")
    sys.exit(1)
print("prerelease detection present and wired to the release action")
PY
[ "$?" -eq 0 ] || fail "prerelease tag semantics"

# 6. The permanent release-verify workflow must be manual-only and read-only:
#    it must not edit the release body, create releases, mutate tags or assets,
#    or silently default the release; it must validate the real GitHub
#    asset-name set and retain the full constrained attestation policy.
wf2="$root/.github/workflows/release-verify.yml"
[ -f "$wf2" ] || fail "release-verify workflow is missing"
python3 - "$wf2" <<'PY'
import sys, yaml, re
d = yaml.safe_load(open(sys.argv[1]))
text = open(sys.argv[1]).read()
issues = []
trig = (d.get("on") or d.get(True)) if isinstance(d, dict) else None
triggers = set(trig.keys()) if isinstance(trig, dict) else set()
if triggers != {"workflow_dispatch"}:
    issues.append(f"release-verify triggers are not manual-only: {sorted(triggers)}")
perm = d.get("permissions") or {}
if perm.get("contents") != "read" or perm.get("attestations") != "read":
    issues.append(f"release-verify permissions are not read-only: {perm}")
if any(v not in ("read", "none") for v in perm.values()):
    issues.append(f"a release-verify permission is write-capable: {perm}")
for bad in "gh release edit", "gh release create", "gh release delete", "gh release upload", "git tag", "gh api --method PATCH":
    if bad in text:
        issues.append(f"release-verify may mutate releases/tags/assets: {bad}")
if ":-v0.1.0" in text or "INPUT_RELEASE:-v0.1.0" in text:
    issues.append("release-verify silently defaults the release to v0.1.0")
# The release must come from the required dispatch input, never a default.
inputs = (trig or {}).get("workflow_dispatch", {}).get("inputs", {})
if not inputs or "release" not in inputs or inputs["release"].get("required") is not True:
    issues.append("release-verify does not require the release input")
for flag in "--repo" "--signer-workflow" "--source-ref" "--source-digest" "--deny-self-hosted-runners":
    if flag not in text:
        issues.append(f"release-verify is missing {flag}")
if "gh attestation verify" not in text:
    issues.append("release-verify does not run gh attestation verify")
if "gh api" not in text or "releases/tags/" not in text or "assets[].name" not in text:
    issues.append("release-verify does not validate the real GitHub asset-name set")
if "sha256sum -c SHA256SUMS" not in text:
    issues.append("release-verify does not verify checksums")
for i in issues:
    print("workflow issue:", i)
if issues:
    sys.exit(1)
print("release-verify workflow is manual-only, read-only, required-input, real-asset-set-validating, full-policy attestation")
PY
[ "$?" -eq 0 ] || fail "release-verify workflow structure"

if [ "$failures" -gt 0 ]; then
  echo "release-contract regression: $failures failure(s)" >&2
  exit 1
fi
echo "release-contract regression passed: workflow wiring, release-notes body (stable/prerelease maturity), asset contract, script agreement, prerelease tag semantics, release-verify policy"
