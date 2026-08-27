#!/bin/sh
set -eu
root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
tmp=$(mktemp -d "${TMPDIR:-/tmp}/trestle-update-test.XXXXXX")
trap 'rm -rf "$tmp"' EXIT INT TERM
mkdir -p "$tmp/home/.local/bin" "$tmp/releases/v9.9.9/payload"
printf '#!/bin/sh\necho old-version\n' > "$tmp/home/.local/bin/trestle"
printf '#!/bin/sh\necho new-version\n' > "$tmp/releases/v9.9.9/payload/trestle"
chmod 0755 "$tmp/home/.local/bin/trestle" "$tmp/releases/v9.9.9/payload/trestle"
tar -czf "$tmp/releases/v9.9.9/trestle_9.9.9_linux_amd64.tar.gz" -C "$tmp/releases/v9.9.9/payload" trestle
(cd "$tmp/releases/v9.9.9" && sha256sum trestle_9.9.9_linux_amd64.tar.gz > SHA256SUMS)
mkdir -p "$tmp/bin"
printf '#!/bin/sh\n[ "${1:-}" = -u ] && echo 1000\n' > "$tmp/bin/id"
chmod 0755 "$tmp/bin/id"
HOME="$tmp/home" PATH="$tmp/bin:$PATH" TRESTLE_RELEASE_BASE="file://$tmp/releases" sh "$root/update.sh" --version v9.9.9
[ "$("$tmp/home/.local/bin/trestle")" = new-version ]
HOME="$tmp/home" PATH="$tmp/bin:$PATH" sh "$root/update.sh" --rollback
[ "$("$tmp/home/.local/bin/trestle")" = old-version ]
echo "update and rollback test passed"
