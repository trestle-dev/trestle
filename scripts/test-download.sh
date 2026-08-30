#!/bin/sh
# CP16 download.sh regression: deterministic tests against a local fake release
# tree (no network, no dependency on GitHub availability). Exercises every
# documented download.sh behavior, the standalone public copy the website
# serves, portable checksum verification, platform mapping, overwrite safety,
# atomic staging cleanup, and a packaged release run as ./trestle version.
set -eu
root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
download="$root/scripts/public/download.sh"
tmp=$(mktemp -d "${TMPDIR:-/tmp}/trestle-download-test.XXXXXX")
trap 'rm -rf "$tmp"' EXIT INT TERM

failures=0
fail() { echo "download test FAIL: $*" >&2; failures=$((failures + 1)); }

# --- Fake release tree ------------------------------------------------------
rel="$tmp/releases"
ver=v0.0.0-test
rel_dir="$rel/$ver"
mkdir -p "$rel_dir/stage"
for p in linux_amd64 linux_arm64 darwin_amd64 darwin_arm64; do
  stage="$rel_dir/stage/trestle_0.0.0-test_$p"
  mkdir -p "$stage"
  printf '#!/bin/sh\necho fake-%s\n' "$p" > "$stage/trestle"
  chmod +x "$stage/trestle"
  tar -C "$rel_dir/stage" -czf "$rel_dir/trestle_0.0.0-test_$p.tar.gz" "trestle_0.0.0-test_$p"
done
(cd "$rel_dir" && sha256sum trestle_0.0.0-test_*.tar.gz > SHA256SUMS)
printf '{"tag_name":"v0.0.0-test"}\n' > "$rel/latest.json"
release_base="file://$rel"

# --- Fake uname + shasum for deterministic platform/tool tests -------------
fakebin="$tmp/fakebin"
mkdir -p "$fakebin"
cat > "$fakebin/uname" <<'EOF'
#!/bin/sh
case "$1" in
  -s) echo "${TRESTLE_TEST_UNAME_S:-Linux}" ;;
  -m) echo "${TRESTLE_TEST_UNAME_M:-x86_64}" ;;
  *) exec /usr/bin/uname "$@" ;;
esac
EOF
cat > "$fakebin/shasum" <<'EOF'
#!/bin/sh
while [ "$#" -gt 0 ]; do case "$1" in -a) shift; shift ;; *) break ;; esac; done
exec sha256sum "$@"
EOF
chmod +x "$fakebin/uname" "$fakebin/shasum"

# --- Helpers -----------------------------------------------------------------
# new_dest sets $dest to a fresh scratch destination directory in the parent
# shell (no command substitution, so the counter persists).
dest_n=0
new_dest() {
  dest_n=$((dest_n + 1))
  dest="$tmp/dest-$dest_n"
  mkdir -p "$dest"
}

# --- 1. Latest-version resolution (via the local release API) ---------------
new_dest
out=$( (cd "$dest" && TRESTLE_RELEASE_BASE="$release_base" TRESTLE_RELEASE_API_URL="file://$rel/latest.json" sh "$download") 2>&1 || true )
echo "$out" | grep -q "version: v0.0.0-test" || fail "latest resolution: got $out"
[ -x "$dest/trestle" ] || fail "latest resolution produced no executable"

# --- 2. Explicit version -----------------------------------------------------
new_dest
out=$( (cd "$dest" && TRESTLE_RELEASE_BASE="$release_base" sh "$download" --version "$ver") 2>&1 )
echo "$out" | grep -q "version: $ver" || fail "explicit version: $out"
[ -x "$dest/trestle" ] || fail "explicit version produced no executable"

# --- 3-6. Platform mapping (fake uname) --------------------------------------
for t in "Linux:x86_64:linux:amd64" "Linux:aarch64:linux:arm64" "Darwin:x86_64:darwin:amd64" "Darwin:arm64:darwin:arm64"; do
  s=${t%%:*}; rest=${t#*:}; m=${rest%%:*}; rest=${rest#*:}; o=${rest%%:*}; a=${rest#*:}
  new_dest
  out=$( (cd "$dest" && TRESTLE_TEST_UNAME_S="$s" TRESTLE_TEST_UNAME_M="$m" PATH="$fakebin:$PATH" TRESTLE_RELEASE_BASE="$release_base" sh "$download" --version "$ver") 2>&1 )
  echo "$out" | grep -q "platform: $o/$a" || fail "$s/$m mapping: $out"
done

# --- 7. Checksum verification with sha256sum (default on Linux) --------------
new_dest
out=$( (cd "$dest" && TRESTLE_RELEASE_BASE="$release_base" sh "$download" --version "$ver") 2>&1 )
echo "$out" | grep -q "^verified trestle_0.0.0-test_linux_amd64.tar.gz " || fail "sha256sum verification: $out"

# --- 8. Checksum verification with shasum -a 256 -----------------------------
new_dest
out=$( (cd "$dest" && PATH="$fakebin:$PATH" TRESTLE_RELEASE_BASE="$release_base" TRESTLE_CHECKSUM_TOOL=shasum sh "$download" --version "$ver") 2>&1 )
echo "$out" | grep -q "^verified trestle_0.0.0-test_linux_amd64.tar.gz " || fail "shasum verification: $out"

# --- 9. Missing checksum file fails closed -----------------------------------
mkdir -p "$rel/nosums/$ver"
tar -C "$rel_dir/stage" -czf "$rel/nosums/$ver/trestle_0.0.0-test_linux_amd64.tar.gz" trestle_0.0.0-test_linux_amd64
new_dest
if (cd "$dest" && TRESTLE_RELEASE_BASE="file://$rel/nosums" sh "$download" --version "$ver" >/dev/null 2>&1); then
  fail "missing SHA256SUMS did not fail closed"
fi

# --- 10. Missing exact archive entry fails closed ----------------------------
mkdir -p "$rel/noentry/$ver"
tar -C "$rel_dir/stage" -czf "$rel/noentry/$ver/trestle_0.0.0-test_linux_amd64.tar.gz" trestle_0.0.0-test_linux_amd64
printf '0000000000000000000000000000000000000000000000000000000000000000  other.tar.gz\n' > "$rel/noentry/$ver/SHA256SUMS"
new_dest
if (cd "$dest" && TRESTLE_RELEASE_BASE="file://$rel/noentry" sh "$download" --version "$ver" >/dev/null 2>&1); then
  fail "missing exact checksum entry did not fail closed"
fi

# --- 11. Malformed checksum entry fails closed --------------------------------
mkdir -p "$rel/malformed/$ver"
tar -C "$rel_dir/stage" -czf "$rel/malformed/$ver/trestle_0.0.0-test_linux_amd64.tar.gz" trestle_0.0.0-test_linux_amd64
printf 'zzz  trestle_0.0.0-test_linux_amd64.tar.gz\n' > "$rel/malformed/$ver/SHA256SUMS"
new_dest
if (cd "$dest" && TRESTLE_RELEASE_BASE="file://$rel/malformed" sh "$download" --version "$ver" >/dev/null 2>&1); then
  fail "malformed checksum entry did not fail closed"
fi

# --- 12. Corrupted archive fails closed ---------------------------------------
mkdir -p "$rel/corrupt/$ver"
tar -C "$rel_dir/stage" -czf "$rel/corrupt/$ver/trestle_0.0.0-test_linux_amd64.tar.gz" trestle_0.0.0-test_linux_amd64
(cd "$rel/corrupt/$ver" && sha256sum trestle_0.0.0-test_linux_amd64.tar.gz > SHA256SUMS)
python3 -c "p='$rel/corrupt/$ver/trestle_0.0.0-test_linux_amd64.tar.gz'; d=open(p,'rb').read(); open(p,'wb').write(d[:-1])"
new_dest
if (cd "$dest" && TRESTLE_RELEASE_BASE="file://$rel/corrupt" sh "$download" --version "$ver" >/dev/null 2>&1); then
  fail "corrupted archive did not fail closed"
fi

# --- 13. Archive missing the expected binary fails closed ---------------------
mkdir -p "$rel/nobin/$ver"
mkdir -p "$rel/nobin/stage/trestle_0.0.0-test_linux_amd64"
printf '#!/bin/sh\necho not-the-binary\n' > "$rel/nobin/stage/trestle_0.0.0-test_linux_amd64/README"
tar -C "$rel/nobin/stage" -czf "$rel/nobin/$ver/trestle_0.0.0-test_linux_amd64.tar.gz" trestle_0.0.0-test_linux_amd64
(cd "$rel/nobin/$ver" && sha256sum trestle_0.0.0-test_linux_amd64.tar.gz > SHA256SUMS)
new_dest
if (cd "$dest" && TRESTLE_RELEASE_BASE="file://$rel/nobin" sh "$download" --version "$ver" >/dev/null 2>&1); then
  fail "archive missing the expected executable did not fail closed"
fi

# --- 14. Existing destination file refusal ------------------------------------
new_dest
printf 'keep me\n' > "$dest/trestle"
if (cd "$dest" && TRESTLE_RELEASE_BASE="$release_base" sh "$download" --version "$ver" >/dev/null 2>&1); then
  fail "existing destination file was not refused"
fi
[ "$(cat "$dest/trestle")" = "keep me" ] || fail "existing destination file was modified"

# --- 15. Existing destination directory refusal --------------------------------
new_dest
mkdir -p "$dest/trestle"
if (cd "$dest" && TRESTLE_RELEASE_BASE="$release_base" sh "$download" --version "$ver" >/dev/null 2>&1); then
  fail "existing destination directory was not refused"
fi
[ -d "$dest/trestle" ] || fail "existing destination directory was removed"

# --- 16. Existing destination symlink refusal ---------------------------------
new_dest
ln -s /dev/null "$dest/trestle"
if (cd "$dest" && TRESTLE_RELEASE_BASE="$release_base" sh "$download" --version "$ver" >/dev/null 2>&1); then
  fail "existing destination symlink was not refused"
fi
[ -L "$dest/trestle" ] || fail "existing destination symlink was replaced without --force"

# --- 17. Explicit --force overwrites a file -----------------------------------
new_dest
printf 'old\n' > "$dest/trestle"
(cd "$dest" && TRESTLE_RELEASE_BASE="$release_base" sh "$download" --force --version "$ver" >/dev/null 2>&1) || fail "--force overwrite failed"
[ -x "$dest/trestle" ] && [ "$(head -c 2 "$dest/trestle")" = "#!" ] || fail "--force did not replace the file"

# --- 18. Custom --output --------------------------------------------------------
new_dest
(cd "$dest" && TRESTLE_RELEASE_BASE="$release_base" sh "$download" --output ./mytrestle --version "$ver" >/dev/null 2>&1) || fail "--output failed"
[ -x "$dest/mytrestle" ] || fail "--output did not create the requested file"
[ ! -e "$dest/trestle" ] || fail "--output also created the default name"

# --- 19. Destination path containing spaces -------------------------------------
new_dest
(cd "$dest" && TRESTLE_RELEASE_BASE="$release_base" sh "$download" --output "my trestle" --version "$ver" >/dev/null 2>&1) || fail "--output with spaces failed"
[ -x "$dest/my trestle" ] || fail "--output with spaces did not create the file"
mkdir -p "$dest/sub dir"
(cd "$dest" && TRESTLE_RELEASE_BASE="$release_base" sh "$download" --output "sub dir/trestle" --version "$ver" >/dev/null 2>&1) || fail "--output in a spaced directory failed"
[ -x "$dest/sub dir/trestle" ] || fail "--output in a spaced directory did not create the file"

# --- 20. Failed operation leaves no partial destination or staging dir --------
new_dest
(cd "$dest" && TRESTLE_RELEASE_BASE="file://$rel/corrupt" sh "$download" --version "$ver" >/dev/null 2>&1) && fail "corrupt download unexpectedly succeeded"
[ ! -e "$dest/trestle" ] || fail "failed download left a partial destination"
[ -z "$(find "$dest" -name '.trestle-download.*' 2>/dev/null)" ] || fail "failed download left a staging directory"

# --- 21. Interrupted operation (SIGTERM) leaves no staging dir -----------------
new_dest
mkdir -p "$fakebin/delay"
cat > "$fakebin/delay/tar" <<'EOF'
#!/bin/sh
sleep 3
exec /bin/tar "$@"
EOF
chmod +x "$fakebin/delay/tar"
( cd "$dest" && PATH="$fakebin/delay:$PATH" TRESTLE_RELEASE_BASE="$release_base" sh "$download" --version "$ver" >/dev/null 2>&1 ) &
pid=$!
sleep 1
kill -TERM "$pid" 2>/dev/null || true
wait "$pid" 2>/dev/null || true
[ ! -e "$dest/trestle" ] || fail "interrupted download left a partial destination"
[ -z "$(find "$dest" -name '.trestle-download.*' 2>/dev/null)" ] || fail "interrupted download left a staging directory"

# --- 22. No PATH / home / service / configuration / data mutation ---------------
home="$tmp/sandbox-home"
mkdir -p "$home"
before_home=$(find "$home" -mindepth 1 | sort)
new_dest
( cd "$dest" && HOME="$home" TRESTLE_RELEASE_BASE="$release_base" sh "$download" --version "$ver" >/dev/null 2>&1 )
after_home=$(find "$home" -mindepth 1 | sort)
[ "$before_home" = "$after_home" ] || fail "download mutated HOME: $after_home"
[ ! -e "$dest/trestle" ] || [ -x "$dest/trestle" ] || fail "destination binary not executable"
[ ! -e /usr/local/bin/trestle ] || fail "download touched /usr/local/bin"

# --- 23. Packaged test release: real binary runs as ./trestle version ---------
pkg="$tmp/pkg"
mkdir -p "$pkg/v1.0.0-test/trestle_1.0.0-test_linux_amd64"
( cd "$root" && go build -o "$pkg/v1.0.0-test/trestle_1.0.0-test_linux_amd64/trestle" ./cmd/trestle )
tar -C "$pkg/v1.0.0-test" -czf "$pkg/v1.0.0-test/trestle_1.0.0-test_linux_amd64.tar.gz" trestle_1.0.0-test_linux_amd64
(cd "$pkg/v1.0.0-test" && sha256sum trestle_1.0.0-test_linux_amd64.tar.gz > SHA256SUMS)
new_dest
(cd "$dest" && TRESTLE_RELEASE_BASE="file://$pkg" sh "$download" --version v1.0.0-test >/dev/null 2>&1) || fail "packaged release download failed"
"$dest/trestle" version >/dev/null 2>&1 || fail "downloaded executable does not run as ./trestle version"

if [ "$failures" -gt 0 ]; then
  echo "download.sh regression: $failures failure(s)" >&2
  exit 1
fi
echo "download.sh regression passed: latest/explicit version, platform mapping, portable verification, fail-closed paths, overwrite safety, atomic staging, no mutation"