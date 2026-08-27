#!/bin/sh
set -eu
output=$(mktemp -d "${TMPDIR:-/tmp}/trestle-artifacts.XXXXXX")
trap 'rm -rf "$output"' EXIT INT TERM
TRESTLE_BUILD_DATE=2026-01-01T00:00:00Z ./scripts/package-release.sh 0.0.0-test "$output"
[ "$(find "$output" -maxdepth 1 -type f | wc -l)" -eq 7 ]
(cd "$output" && sha256sum -c SHA256SUMS)
for archive in "$output"/*.tar.gz; do tar -tzf "$archive" | grep -E '/trestle$' >/dev/null; done
for archive in "$output"/*.zip; do unzip -l "$archive" | grep -E '/trestle.exe$' >/dev/null; done
mkdir -p "$output/verify"
tar -xzf "$output/trestle_0.0.0-test_linux_amd64.tar.gz" -C "$output/verify"
"$output/verify/trestle_0.0.0-test_linux_amd64/trestle" version | grep -q '"version":"0.0.0-test"'
"$output/verify/trestle_0.0.0-test_linux_amd64/trestle" version | grep -q '"date":"2026-01-01T00:00:00Z"'
