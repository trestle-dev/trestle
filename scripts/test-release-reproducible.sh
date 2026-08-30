#!/bin/sh
# CP15 reproducibility regression: two independent packaging runs from the same
# commit, in the same environment, must produce byte-identical archives and an
# identical SHA256SUMS.
#
# Scope: byte-identity is proven for the same source, inputs, Go toolchain and
# packaging environment (GNU tar + Info-ZIP, as in the pinned Ubuntu release
# job). It is not a claim across arbitrary Go, tar, ZIP or OS versions.
# package-release.sh fails closed when the GNU tar toolchain is unavailable.
#
# The embedded build date is derived from the commit (never the wall clock), Go
# VCS stamping is disabled, tar archives use deterministic ordering/ownership/
# mtimes, and zip archives use normalized mtimes with stable ordering.
set -eu
root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
tmp=$(mktemp -d "${TMPDIR:-/tmp}/trestle-repro.XXXXXX")
trap 'rm -rf "$tmp"' EXIT INT TERM

( cd "$root" && git diff --quiet && git diff --cached --quiet ) || {
  echo "reproducibility regression requires a clean working tree (the packaged README/LICENSE and VCS inputs must be stable)" >&2
  exit 1
}

./scripts/package-release.sh 0.0.0-repro "$tmp/one" >/dev/null
./scripts/package-release.sh 0.0.0-repro "$tmp/two" >/dev/null

failures=0
cmp -s "$tmp/one/SHA256SUMS" "$tmp/two/SHA256SUMS" \
  || { echo "SHA256SUMS differ between the two runs" >&2; failures=$((failures + 1)); }
for f in "$tmp"/one/*; do
  b=$(basename "$f")
  cmp -s "$f" "$tmp/two/$b" || { echo "archive differs between runs: $b" >&2; failures=$((failures + 1)); }
done
[ "$(find "$tmp/one" -maxdepth 1 -type f | wc -l)" -eq 7 ] \
  || { echo "expected 7 release files, got $(find "$tmp/one" -maxdepth 1 -type f | wc -l)" >&2; failures=$((failures + 1)); }

if [ "$failures" -gt 0 ]; then
  echo "release reproducibility regression: $failures failure(s)" >&2
  exit 1
fi
echo "release reproducibility regression passed: two independent runs produced byte-identical archives and SHA256SUMS"