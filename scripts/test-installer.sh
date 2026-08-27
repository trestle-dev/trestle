#!/bin/sh
set -eu
root=$(mktemp -d "${TMPDIR:-/tmp}/trestle-installer.XXXXXX")
trap 'rm -rf "$root"' EXIT INT TERM
mkdir -p "$root/bin" "$root/releases/v0.0.0-test" "$root/stage/trestle_0.0.0-test_linux_amd64"
printf '#!/bin/sh\necho test-release\n' > "$root/stage/trestle_0.0.0-test_linux_amd64/trestle"
chmod +x "$root/stage/trestle_0.0.0-test_linux_amd64/trestle"
tar -C "$root/stage" -czf "$root/releases/v0.0.0-test/trestle_0.0.0-test_linux_amd64.tar.gz" trestle_0.0.0-test_linux_amd64
printf '#!/bin/sh\necho 1000\n' > "$root/bin/id"
chmod +x "$root/bin/id"
PATH="$root/bin:$PATH" TRESTLE_RELEASE_BASE="file://$root/releases" TRESTLE_INSTALL_DIR="$root/install" ./install.sh --version v0.0.0-test
[ "$($root/install/trestle)" = test-release ]
if PATH="$root/bin:$PATH" ./install.sh --system >/dev/null 2>&1; then
  echo "non-root system install unexpectedly succeeded" >&2
  exit 1
fi
