# Trestle v{{VERSION}} (preview release candidate)

This release is a **preview / release candidate**, not a stable release.
Compatibility is frozen only when a stable version is published; a
release-candidate defect may still require a documented breaking correction.
Before adopting it for production, read the candidate support boundary
(`STABILITY.md`) and hardening evidence.

## What this release is

- One self-hosted Go executable with an embedded administration dashboard, a
  versioned HTTP/OpenAPI boundary, and SQLite or PostgreSQL storage.
- Typed collections, application authentication, access rules, files, realtime
  SSE, audit, durable jobs, signed webhooks, AWS Lambda delivery, backups and
  offline restore.
- Checksum-verified release archives for Linux, macOS and Windows on amd64 and
  arm64, plus `SHA256SUMS`.

## Supported database options

- **SQLite** is the embedded default. One Trestle process owns one local SQLite
  database; SQLite on a shared/network filesystem is not supported.
- **PostgreSQL 16, 17 or 18** is the externally operated option, on a
  single-process ownership model. PostgreSQL and SQLite share the same typed
  collections, transactions, rules, queries and API contracts; offline
  cross-provider migration is supported. Managed/serverless PostgreSQL
  topologies beyond the tested single-process model are outside the contract.

## Operator responsibilities

- Trestle binds loopback by default. TLS termination, trusted-proxy
  configuration (so forwarded scheme and client identity come only from
  configured CIDRs), firewalling, owner-only configuration/data permissions,
  provider-managed S3 recovery and host patching are operator responsibilities.
- **Back up before upgrading.** The updater replaces only the executable,
  verifies the archive against `SHA256SUMS` before writing, retains the previous
  binary for rollback (`update.sh --rollback`), and never modifies instance data
  or configuration. Schema upgrades are one-way: there is no automatic database
  downgrade during executable rollback.

## Known limitations

- No automatic database downgrade during executable rollback.
- Multiple Trestle processes sharing one database are not supported.
- No clustering, GraphQL, plugin marketplace, managed cloud, official container
  image, outbound verification email or self-service password recovery in this
  release.
- Before stable release, only the newest release line receives security fixes.

## Verified installation

Install on this user account (`~/.local/bin`), download into the current
directory (only `./trestle`), or update an existing install - all three verify
the archive checksum and never execute the downloaded binary:

```sh
curl -fsSL https://trestle.cv/install.sh | sh
curl -fsSL https://trestle.cv/download.sh | sh
./trestle
curl -fsSL https://trestle.cv/update.sh | sh
```

Pin any release with `--version vX.Y.Z`; download also supports `--output` and
`--force`.

## Changelog

Automatically generated notes follow.