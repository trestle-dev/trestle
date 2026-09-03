# Trestle release runbook (human-controlled)

This is the exact human-controlled sequence for publishing a tagged release and
verifying it. It is intentionally manual: the tag, the inspection and the
public smoke test are decisions a person makes, not automation.

Every release is an evidence gate (see `HANDOVER.md`). If `install.sh`,
`download.sh` or `update.sh` changed since the previous release, their exact
reviewed versions must be deployed and byte-verified on the website BEFORE this
runbook starts (public-script parity gate: `scripts/test-public-scripts.sh`).

## Version under release

Set and validate the exact tag being released before any command in this
runbook; every example uses it:

```sh
export VERSION="0.1.2"        # MAJOR.MINOR.PATCH, optionally -prerelease.suffix
printf '%s' "$VERSION" | LC_ALL=C grep -Eq '^[0-9]+[.][0-9]+[.][0-9]+(-[0-9A-Za-z]+([.][0-9A-Za-z]+)*)?$' \
  || { echo "invalid VERSION '$VERSION'" >&2; exit 1; }
case "$VERSION" in
  *-*) echo "releasing prerelease tag v$VERSION (GitHub prerelease)" ;;
  *)   echo "releasing stable tag v$VERSION (normal GitHub release, stable public preview)" ;;
esac
```

The validation accepts exactly a stable `MAJOR.MINOR.PATCH` or a prerelease
`MAJOR.MINOR.PATCH-<identifier>` (with optional dot-separated identifiers). It
rejects malformed values including a trailing separator, an empty prerelease
identifier, or anything after the numeric core other than a `-`-prefixed
identifier. The tag is always `v$VERSION`. A stable `vMAJOR.MINOR.PATCH` tag is
a normal GitHub release with stable public-preview product maturity; it is never
itself a release candidate and makes no production-proven or battle-proven
claim. A tag containing a `-` suffix (for example `v0.1.0-rc.1`) is a GitHub
prerelease that honestly describes itself as a preview / release candidate.

## Preconditions

- Application CI is green on `main` (PostgreSQL 16/17/18 suites and the gate).
- `./scripts/test-public-scripts.sh` passes (canonical public scripts match the
  website source and generated copies byte-for-byte).
- `./scripts/test-release.sh`, `./scripts/test-release-reproducible.sh` and
  `./scripts/test-release-contract.sh` pass (six archives, checksums, layout,
  reproducibility, workflow/notes contract).
- `./scripts/test-download.sh`, `./scripts/test-installer.sh`,
  `./scripts/test-update.sh` pass against local fake release assets.
- The release notes template (`docs/release-notes-template.md`) produces the
  correct maturity wording for the release kind: a stable tag leads with
  "stable public preview" and never calls itself a release candidate; a
  prerelease tag honestly says it is a preview / release candidate, not a
  stable release. The template retains supported PostgreSQL versions,
  SQLite/PostgreSQL boundaries, TLS and operator responsibilities,
  backup-before-upgrade, known limitations and verification instructions.
- The website status matches the release kind (a stable release is described
  as a public preview, not battle-proven; an unreleased state says no stable
  version is published).

### Bind the tag to the reviewed commit (human)

The tag must be created from the exact remote head that was reviewed. Before
tagging:

```sh
git fetch origin
test "$(git rev-parse HEAD)" = "$(git rev-parse origin/main)"
git status --porcelain
git show -s --format='%H %s' HEAD
```

Record that exact commit in the release evidence. After creating the annotated
tag and before pushing it:

```sh
git rev-list -n 1 "v${VERSION}"
```

must equal the recorded reviewed commit.

## Sequence

### 1. Tag (human)

Create the annotated tag locally and push it. The annotation follows the
release kind determined above: a stable version is tagged as a stable public
preview, a prerelease version is tagged honestly as a preview / release
candidate:

```sh
case "$VERSION" in
  *-*) annotation="Trestle v${VERSION} (preview / release candidate)" ;;
  *)   annotation="Trestle v${VERSION} (stable public preview)" ;;
esac
git tag -a "v${VERSION}" -m "$annotation"
git push origin "v${VERSION}"
```

Pushing a `v*` tag triggers `.github/workflows/release.yml` only. No other
action happens automatically.

### 1b. Release-kind decision (recorded)

A stable semver tag (`vMAJOR.MINOR.PATCH`) is released as a **normal,
non-prerelease** GitHub release so `/releases/latest` selects it, and its
release body leads with "stable public preview". Stable public-preview maturity
means the release establishes the normal installable download channel and the
initial public compatibility contract; it is not a production-proven or
battle-proven claim, and a stable tag is never itself described as a release
candidate. A prerelease tag (`vX.Y.Z-*`) is a GitHub prerelease that honestly
calls itself a preview / release candidate. This matches the release-note
wording validation in `.github/workflows/release.yml`.

### 2. Actions completion (observe, do not cut short)

The `Release` workflow runs: Verify, Package six targets, Write release notes,
attest build provenance, create the release. Wait for the workflow to complete
successfully. The workflow:

- packages `trestle_<version>_<os>_<arch>.tar.gz` for
  `linux/amd64 linux/arm64 darwin/amd64 darwin/arm64` and `.zip` for
  `windows/amd64 windows/arm64`;
- writes `SHA256SUMS` (one line per archive: 64 lowercase hex, two spaces, exact
  filename) and uploads all seven files as release assets;
- creates a normal release (never draft, never prerelease) with the release
  notes template body plus the auto-generated changelog. For a stable tag the
  body leads with "stable public preview" and never calls itself a release
  candidate or "not a stable release"; for a prerelease tag it honestly says
  it is a preview / release candidate, not a stable release.

### 3. Asset inspection (human)

Verify on the release page or via the API:

- All six archives and `SHA256SUMS` are attached to `v${VERSION}`.
- `SHA256SUMS -c` passes against the downloaded archives.
- Each `.tar.gz` contains exactly `trestle_<version>_<os>_<arch>/trestle`.
- The release kind matches the tag: a stable version is a normal release
  (`draft=false`, `prerelease=false`) so `/releases/latest` selects it; a
  prerelease version is `draft=false`, `prerelease=true` so it stays out of
  `latest` resolution and must be selected explicitly by version. A draft
  would be excluded from `latest` in either case.

### 4. Hosted-script verification (human)

Download the live website scripts and byte-compare them with the canonical
copies, then run the actual GitHub release download path:

```sh
for s in install download update; do
  curl -fsSL "https://trestle.cv/$s.sh" | cmp - <(cat "scripts/public/$s.sh")
done
```

Smoke the real asset URLs (checksum-verified by the scripts themselves):

```sh
mkdir smoke && cd smoke
curl -fsSL https://trestle.cv/download.sh | sh
./trestle version
```

Also verify a pinned install and an update from the real release:

```sh
curl -fsSL https://trestle.cv/install.sh | sh -s -- --version "v${VERSION}"
curl -fsSL https://trestle.cv/update.sh | sh -s -- --version "v${VERSION}"
```

### 5. Rollback (human)

If the release is defective:

1. Delete the GitHub release (keeps the tag) or the tag itself
   (`git tag -d "v${VERSION}" && git push origin ":refs/tags/v${VERSION}"`).
2. For operators who already updated: `update.sh --rollback` restores the
   previous executable (binary-only; data schema upgrades are one-way and
   must be recovered with a pre-upgrade backup/restore).

**`latest` after deleting a release depends on whether an earlier normal
release exists:**

- If an earlier normal release exists, `/releases/latest` falls back to it and
  the unpinned public commands keep working.
- If no earlier normal release exists (the common case for a first release),
  `/releases/latest` has no eligible release: the default unpinned
  `download.sh`, `install.sh` and `update.sh` commands are **unavailable**
  until another normal release exists. Do not imply that deleting a first
  release restores a usable public download channel.
- Deleting a published release also makes its **pinned** asset URLs
  (`/releases/download/v${VERSION}/...`) unavailable, so `--version v${VERSION}` fails
  too.
- Existing installations can still use their retained local binary rollback
  (`update.sh --rollback`), but that is a local binary-only operation: schema
  compatibility with older data and pre-upgrade backups remain separate
  concerns. The website scripts are untouched and remain the previous, verified
  state.

### Stop conditions

Do not announce Trestle publicly, and do not treat `latest` as authoritative
for anything beyond the previous step, until a separate human review of the
published release, hosted scripts and smoke results has been completed.