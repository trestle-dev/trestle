#!/bin/sh
# CP16 update.sh regression: portable checksum verification (sha256sum and the
# shasum -a 256 fallback), atomic replace with rollback, and fail-closed update
# when the release is unverifiable. Uses a local fake release tree.
set -eu
root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
tmp=$(mktemp -d "${TMPDIR:-/tmp}/trestle-update-test.XXXXXX")
trap 'rm -rf "$tmp"' EXIT INT TERM
mkdir -p "$tmp/home/.local/bin" "$tmp/releases/v9.9.9/payload/trestle_9.9.9_linux_amd64"
printf '#!/bin/sh\necho old-version\n' > "$tmp/home/.local/bin/trestle"
printf '#!/bin/sh\necho new-version\n' > "$tmp/releases/v9.9.9/payload/trestle_9.9.9_linux_amd64/trestle"
chmod 0755 "$tmp/home/.local/bin/trestle" "$tmp/releases/v9.9.9/payload/trestle_9.9.9_linux_amd64/trestle"
tar -C "$tmp/releases/v9.9.9/payload" -czf "$tmp/releases/v9.9.9/trestle_9.9.9_linux_amd64.tar.gz" trestle_9.9.9_linux_amd64
(cd "$tmp/releases/v9.9.9" && sha256sum trestle_9.9.9_linux_amd64.tar.gz > SHA256SUMS)
mkdir -p "$tmp/bin"
printf '#!/bin/sh\n[ "${1:-}" = -u ] && echo 1000\n' > "$tmp/bin/id"
chmod 0755 "$tmp/bin/id"

# 1. Portable verification via shasum -a 256 (the macOS fallback path), then
# update and rollback.
HOME="$tmp/home" PATH="$tmp/bin:$PATH" TRESTLE_RELEASE_BASE="file://$tmp/releases" TRESTLE_CHECKSUM_TOOL=shasum sh "$root/update.sh" --version v9.9.9
[ "$("$tmp/home/.local/bin/trestle")" = new-version ]
HOME="$tmp/home" PATH="$tmp/bin:$PATH" sh "$root/update.sh" --rollback
[ "$("$tmp/home/.local/bin/trestle")" = old-version ]

# 2. Default sha256sum path also updates successfully.
HOME="$tmp/home" PATH="$tmp/bin:$PATH" TRESTLE_RELEASE_BASE="file://$tmp/releases" sh "$root/update.sh" --version v9.9.9
[ "$("$tmp/home/.local/bin/trestle")" = new-version ]

# 3. A release without SHA256SUMS fails closed and leaves the binary untouched.
mkdir -p "$tmp/releases/unverified/v9.9.9"
tar -C "$tmp/releases/v9.9.9/payload" -czf "$tmp/releases/unverified/v9.9.9/trestle_9.9.9_linux_amd64.tar.gz" trestle_9.9.9_linux_amd64
if HOME="$tmp/home" PATH="$tmp/bin:$PATH" TRESTLE_RELEASE_BASE="file://$tmp/releases/unverified" sh "$root/update.sh" --version v9.9.9 >/dev/null 2>&1; then
  echo "update from an unverified release unexpectedly succeeded" >&2
  exit 1
fi
[ "$("$tmp/home/.local/bin/trestle")" = new-version ] || { echo "failed update changed the binary" >&2; exit 1; }
echo "update and rollback test passed: portable verification, atomic replace, fail-closed"