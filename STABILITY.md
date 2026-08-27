# Release-candidate contract

CP23 closes the planned implementation and hardening programme. This commit is a
release candidate, not a published stable release.

## Candidate-supported surfaces

- Linux and macOS on amd64 and arm64, and Windows on amd64 and arm64.
- One Trestle process owning one local SQLite database on a local filesystem.
- The versioned `/api/v1`, `/admin/v1`, and `/system` HTTP contracts documented
  by the bundled OpenAPI document and public documentation.
- Application users, administrator sessions, scoped service credentials,
  collection rules, typed records, local and S3-compatible files, SSE, jobs,
  audit, signed webhooks, AWS Lambda delivery, backups and offline restore.
- Checksum-verified archives, user installs, updates and executable rollback.

## Explicitly outside the candidate contract

- Multi-writer clustering or SQLite on a shared/network filesystem.
- Automatic database downgrade during executable rollback.
- Direct application-user file delivery through collection rules.
- Target-aware relation foreign keys or automatic relation expansion.
- Outbound email verification and self-service password recovery.
- An official container image or managed service.

Compatibility is frozen only when a stable version is published. Until then,
release-candidate findings may still require a documented breaking correction.
