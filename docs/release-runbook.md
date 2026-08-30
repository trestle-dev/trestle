# Trestle release runbook (human-controlled)

This is the exact human-controlled sequence for publishing a tagged release and
verifying it. It is intentionally manual: the tag, the inspection and the
public smoke test are decisions a person makes, not automation.

Every release is an evidence gate (see `HANDOVER.md`). If `install.sh`,
`download.sh` or `update.sh` changed since the previous release, their exact
reviewed versions must be deployed and byte-verified on the website BEFORE this
runbook starts (public-script parity gate: `scripts/test-public-scripts.sh`).

## Preconditions

- Application CI is green on `main` (PostgreSQL 16/17/18 suites and the gate).
- `./scripts/test-public-scripts.sh` passes (canonical public scripts match the
  website source and generated copies byte-for-byte).
- `./scripts/test-release.sh` passes (six archives, checksums, layout,
  version/date injection).
- `./scripts/test-download.sh`, `./scripts/test-installer.sh`,
  `./scripts/test-update.sh` pass against local fake release assets.
- The release notes template (`docs/release-notes-template.md`) states preview
  status, supported PostgreSQL versions, SQLite/PostgreSQL boundaries, TLS and
  operator responsibilities, backup-before-upgrade, and known limitations.
- The website does not claim a stable release already exists.

## Sequence

### 1. Tag (human)

Create the annotated tag locally and push it:

```sh
git tag -a v0.1.0 -m "Trestle v0.1.0 (preview release candidate)"
git push origin v0.1.0
```

Pushing a `v*` tag triggers `.github/workflows/release.yml` only. No other
action happens automatically.

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
  notes template body plus the auto-generated changelog.

### 3. Asset inspection (human)

Verify on the release page or via the API:

- All six archives and `SHA256SUMS` are attached to `v0.1.0`.
- `SHA256SUMS -c` passes against the downloaded archives.
- Each `.tar.gz` contains exactly `trestle_<version>_<os>_<arch>/trestle`.
- The release is a normal release (`draft=false`, `prerelease=false`), so
  `/releases/latest` will select it. A draft or prerelease would be excluded
  from `latest` resolution.

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
curl -fsSL https://trestle.cv/install.sh | sh -s -- --version v0.1.0
curl -fsSL https://trestle.cv/update.sh | sh -s -- --version v0.1.0
```

### 5. Rollback (human)

If the release is defective:

1. Delete the GitHub release (keeps the tag) or the tag itself
   (`git tag -d v0.1.0 && git push origin :refs/tags/v0.1.0`).
2. For operators who already updated: `update.sh --rollback` restores the
   previous executable (binary-only; data schema upgrades are one-way and
   must be recovered with a pre-upgrade backup/restore).
3. The website scripts are untouched and remain the previous, verified state;
   `latest` resolution returns the previous stable release.

### Stop conditions

Do not announce Trestle publicly, and do not treat `latest` as authoritative
for anything beyond the previous step, until a separate human review of the
published release, hosted scripts and smoke results has been completed.