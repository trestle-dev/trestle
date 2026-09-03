# Stability and support contract

Trestle's v0.1.x stable public-preview releases establish the normal,
installable download channel and the initial public compatibility contract. A
stable public-preview release is not a production-proven or battle-proven
claim, and a stable tag is never itself a release candidate. The v0.1.x
contract is young: later stable releases within the line may still require
documented corrections while the product gains field evidence.

## Supported surfaces (v0.1.x contract)

- Linux and macOS on amd64 and arm64, and Windows on amd64 and arm64.
- One Trestle process owning one configured database: an owned local SQLite
  database on a local filesystem, or an external PostgreSQL 16, 17 or 18
  service.
- The versioned `/api/v1`, `/admin/v1`, and `/system` HTTP contracts documented
  by the bundled OpenAPI document and public documentation.
- Application users, administrator sessions, scoped service credentials,
  collection rules, typed records, local and S3-compatible files, SSE, jobs,
  audit, signed webhooks, AWS Lambda delivery, backups and offline restore.
- Cross-provider migration between SQLite and PostgreSQL, with provider-specific
  backup and restore procedures.
- Checksum-verified archives, user installs, executable updates and explicit
  rollback.

## Explicitly outside the v0.1.x contract

- Multiple Trestle processes sharing one database, or SQLite on a
  shared/network filesystem.
- Managed/serverless PostgreSQL topologies beyond the tested single-process
  ownership model.
- Automatic database downgrade during executable rollback.
- Direct application-user file delivery through collection rules.
- Target-aware relation foreign keys or automatic relation expansion.
- Outbound email verification and self-service password recovery.
- An official container image or managed service.
