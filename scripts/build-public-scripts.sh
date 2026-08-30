#!/bin/sh
# Regenerates the standalone public script copies served by the website.
#
# Each canonical repository script (install.sh, download.sh, update.sh) sources
# scripts/checksum.sh from a repository checkout. Public consumers run the
# scripts via `curl ... | sh`, which must not depend on a local helper file, so
# this build emits a self-contained copy with checksum.sh inlined at the top and
# the two repository-only source lines removed.
#
# The generated files are committed at scripts/public/ and the parity gate
# (scripts/test-public-scripts.sh) refuses a diff between the regenerated output
# and the committed copies, so there is exactly one hand-maintained source per
# public script.
set -eu
root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
out="$root/scripts/public"
mkdir -p "$out"

for name in install download update; do
  {
    cat "$root/scripts/checksum.sh"
    printf '\n'
    sed -e '/^root_dir=\$(CDPATH= cd -- "\$(dirname "\$0")" && pwd)$/d' \
        -e '/^\. "\$root_dir\/scripts\/checksum\.sh"$/d' \
        "$root/$name.sh"
  } > "$out/$name.sh"
  chmod 0755 "$out/$name.sh"
done
echo "regenerated public script copies in $out"