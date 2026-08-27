#!/bin/sh
set -eu

version=${1:?usage: package-release.sh VERSION OUTPUT_DIR}
output_dir=${2:?usage: package-release.sh VERSION OUTPUT_DIR}
commit=${TRESTLE_COMMIT:-$(git rev-parse HEAD)}
build_date=${TRESTLE_BUILD_DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}
ldflags="-s -w -X github.com/trestle-dev/trestle/internal/buildinfo.version=$version -X github.com/trestle-dev/trestle/internal/buildinfo.commit=$commit -X github.com/trestle-dev/trestle/internal/buildinfo.date=$build_date"

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
  CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -trimpath -ldflags "$ldflags" -o "$stage/$binary" ./cmd/trestle
  cp README.md LICENSE "$stage/"
  if [ "$os" = windows ]; then
    (cd "$work" && zip -qr "$output_dir/$name.zip" "$name")
  else
    tar -C "$work" -czf "$output_dir/$name.tar.gz" "$name"
  fi
done
(cd "$output_dir" && sha256sum trestle_* > SHA256SUMS)
