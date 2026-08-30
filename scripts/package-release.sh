#!/bin/sh
set -eu

# Deterministic release packaging (CP15).
#
# Two runs from the same commit, in the same environment, produce byte-identical
# archives and identical SHA256SUMS:
#   - the embedded build date comes from TRESTLE_BUILD_DATE, else
#     SOURCE_DATE_EPOCH, else the tagged/HEAD commit's committer timestamp
#     (never the wall clock), so the binary is reproducible;
#   - the build excludes Go's VCS stamping (-buildvcs=false); the commit is
#     injected explicitly via ldflags;
#   - tar archives use SOURCE_DATE_EPOCH with sorted, numeric-owner entries and
#     no archive timestamp;
#   - zip archives use -X (no extra fields) with mtimes pinned to
#     SOURCE_DATE_EPOCH (Info-ZIP does not honor the variable itself) and
#     stable entry ordering.
#
# Scope: byte-identical output is guaranteed for the same source, inputs, Go
# toolchain and packaging environment - the pinned Ubuntu release job with GNU
# tar and Info-ZIP. It is NOT a claim of byte-identity across arbitrary Go,
# tar, ZIP or operating-system versions. Packaging therefore fails closed when
# the required GNU tar toolchain is unavailable; local macOS packaging is
# unsupported rather than pretending to be reproducible.
#
# scripts/test-release-reproducible.sh runs packaging twice from the same
# commit and requires byte-identical archives and identical SHA256SUMS.
#
# usage: package-release.sh VERSION OUTPUT_DIR
version=${1:?usage: package-release.sh VERSION OUTPUT_DIR}
output_dir=${2:?usage: package-release.sh VERSION OUTPUT_DIR}
commit=${TRESTLE_COMMIT:-$(git rev-parse HEAD)}

if ! tar --version 2>/dev/null | grep -q GNU; then
  echo "deterministic release packaging requires GNU tar (the pinned Ubuntu release job)" >&2
  echo "unsupported toolchain: local macOS packaging is not supported; use the release workflow or a GNU/Linux environment" >&2
  exit 1
fi
command -v zip >/dev/null 2>&1 || { echo "Info-ZIP zip is required for the Windows archives" >&2; exit 1; }

format_epoch() {
  # Portable UTC ISO8601 from an epoch value (GNU and BSD date).
  if date -u -d "@$1" >/dev/null 2>&1; then
    date -u -d "@$1" +%Y-%m-%dT%H:%M:%SZ
  else
    date -u -r "$1" +%Y-%m-%dT%H:%M:%SZ
  fi
}

if [ -z "${SOURCE_DATE_EPOCH:-}" ]; then
  SOURCE_DATE_EPOCH=$(git show -s --format=%ct "$commit" 2>/dev/null || echo 0)
fi
export SOURCE_DATE_EPOCH
if [ -n "${TRESTLE_BUILD_DATE:-}" ]; then
  build_date=$TRESTLE_BUILD_DATE
else
  build_date=$(format_epoch "$SOURCE_DATE_EPOCH")
fi
ldflags="-s -w -X github.com/trestle-dev/trestle/internal/buildinfo.version=$version -X github.com/trestle-dev/trestle/internal/buildinfo.commit=$commit -X github.com/trestle-dev/trestle/internal/buildinfo.date=$build_date"

# normalize_mtimes PATH: pins every mtime under PATH to SOURCE_DATE_EPOCH so
# Info-ZIP (which does not honor SOURCE_DATE_EPOCH) stores a fixed DOS time.
normalize_mtimes() {
  if date -u -d "@0" >/dev/null 2>&1; then
    find "$1" -exec touch -d "@$SOURCE_DATE_EPOCH" {} +
  else
    ts=$(date -u -r "$SOURCE_DATE_EPOCH" +%Y%m%d%H%M.%S)
    find "$1" -exec touch -t "$ts" {} +
  fi
}

mkdir -p "$output_dir"
output_dir=$(cd "$output_dir" && pwd)
work=$(mktemp -d "${TMPDIR:-/tmp}/trestle-release.XXXXXX")
trap 'rm -rf "$work"' EXIT INT TERM

for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64; do
  os=${target%/*}
  arch=${target#*/}
  name="trestle_${version}_${os}_${arch}"
  stage="$work/$name"
  mkdir -p "$stage"
  binary=trestle
  [ "$os" = windows ] && binary=trestle.exe
  CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -trimpath -buildvcs=false -ldflags "$ldflags" -o "$stage/$binary" ./cmd/trestle
  cp README.md LICENSE "$stage/"
  if [ "$os" = windows ]; then
    normalize_mtimes "$stage"
    (cd "$work" && find "$name" -print | LC_ALL=C sort | zip -q -X -@ "$output_dir/$name.zip")
  else
    tar -C "$work" --sort=name --numeric-owner --owner=0 --group=0 --mtime="@$SOURCE_DATE_EPOCH" --format=ustar -czf "$output_dir/$name.tar.gz" "$name"
  fi
done
(cd "$output_dir" && sha256sum trestle_* > SHA256SUMS)