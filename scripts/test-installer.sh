#!/bin/sh
# CP16 install.sh regression: verifies the release checksum before writing,
# installs into an explicit directory, and fails closed when the release is
# unverifiable. Uses a local fake release tree (no network). The script under
# test is the canonical repository script, which sources scripts/checksum.sh;
# the parity gate proves the standalone public copy is equivalent.
set -eu
root=$(mktemp -d "${TMPDIR:-/tmp}/trestle-installer.XXXXXX")
trap 'rm -rf "$root"' EXIT INT TERM
mkdir -p "$root/bin" "$root/releases/v0.0.0-test/stage/trestle_0.0.0-test_linux_amd64"
printf '#!/bin/sh\necho test-release\n' > "$root/releases/v0.0.0-test/stage/trestle_0.0.0-test_linux_amd64/trestle"
chmod +x "$root/releases/v0.0.0-test/stage/trestle_0.0.0-test_linux_amd64/trestle"
tar -C "$root/releases/v0.0.0-test/stage" -czf "$root/releases/v0.0.0-test/trestle_0.0.0-test_linux_amd64.tar.gz" trestle_0.0.0-test_linux_amd64
(cd "$root/releases/v0.0.0-test" && sha256sum trestle_0.0.0-test_linux_amd64.tar.gz > SHA256SUMS)
printf '#!/bin/sh\necho 1000\n' > "$root/bin/id"
chmod +x "$root/bin/id"

cd "$(dirname "$0")/.."

# 1. Verifies before installing and installs into the explicit directory.
PATH="$root/bin:$PATH" TRESTLE_RELEASE_BASE="file://$root/releases" TRESTLE_INSTALL_DIR="$root/install" sh install.sh --version v0.0.0-test
[ "$("$root/install/trestle")" = test-release ]

# 2. A release without SHA256SUMS fails closed before anything is installed.
mkdir -p "$root/releases/unverified/v0.0.0-test"
tar -C "$root/releases/v0.0.0-test/stage" -czf "$root/releases/unverified/v0.0.0-test/trestle_0.0.0-test_linux_amd64.tar.gz" trestle_0.0.0-test_linux_amd64
if PATH="$root/bin:$PATH" TRESTLE_RELEASE_BASE="file://$root/releases/unverified" TRESTLE_INSTALL_DIR="$root/install2" sh install.sh --version v0.0.0-test >/dev/null 2>&1; then
  echo "install of an unverified release unexpectedly succeeded" >&2
  exit 1
fi
[ ! -e "$root/install2/trestle" ] || { echo "install wrote a binary without verifying" >&2; exit 1; }

# 3. A corrupted archive fails checksum verification and installs nothing.
mkdir -p "$root/releases/corrupt/v0.0.0-test"
tar -C "$root/releases/v0.0.0-test/stage" -czf "$root/releases/corrupt/v0.0.0-test/trestle_0.0.0-test_linux_amd64.tar.gz" trestle_0.0.0-test_linux_amd64
(cd "$root/releases/corrupt/v0.0.0-test" && sha256sum trestle_0.0.0-test_linux_amd64.tar.gz > SHA256SUMS)
python3 -c "p='$root/releases/corrupt/v0.0.0-test/trestle_0.0.0-test_linux_amd64.tar.gz'; d=open(p,'rb').read(); open(p,'wb').write(d[:-1])"
if PATH="$root/bin:$PATH" TRESTLE_RELEASE_BASE="file://$root/releases/corrupt" TRESTLE_INSTALL_DIR="$root/install3" sh install.sh --version v0.0.0-test >/dev/null 2>&1; then
  echo "install of a corrupted archive unexpectedly succeeded" >&2
  exit 1
fi
[ ! -e "$root/install3/trestle" ] || { echo "install wrote a corrupted binary" >&2; exit 1; }

# 4. Non-root system install is refused.
if PATH="$root/bin:$PATH" sh install.sh --system >/dev/null 2>&1; then
  echo "non-root system install unexpectedly succeeded" >&2
  exit 1
fi
echo "installer test passed: verifies before installing, fails closed on unverifiable releases"