# Security model

This is an engineering contract and threat model, not a claim that failure is
impossible or a substitute for independent assessment.

## Trust zones

- Untrusted clients: browsers, mobile apps, public API callers, uploaded files,
  filter/rule text, and realtime subscription input.
- Trusted application backends: authenticated by scoped service identities, not
  automatically trusted because of network location.
- Administrators: scoped administrative identities; superuser is exceptional.
- Trestle process and its configured database (SQLite file or PostgreSQL
  service): trusted computing boundary for one deployment.
- External systems: object stores, proxies, webhooks, Lambda, email, and identity
  providers; responses and callbacks remain untrusted input.

## Identity and least authority

Keep user sessions, service accounts, personal tokens, function grants, and
superusers distinct in storage, token audience, middleware, audit, and UI. Tokens
are hashed where lookup permits, short-lived where practical, revocable, rotated,
and never logged. Bootstrap has no durable default password or public setup race.

Application backends use scoped service accounts. Ordinary SDK/API usage must
never require a superuser token. Function grants bind to one audience and narrow
callback capability. Every mutation and sensitive read records an attributable
actor and request ID.

## Authorization invariants

Collection rules execute server-side for reads, writes, deletes, relation
expansion, batches, files, and realtime. A list rule cannot be bypassed through
record lookup, relation expansion, file URL knowledge, event replay, aggregate
metadata, or error differences. Realtime rechecks current authority at delivery.

Rules and filters are parsed, typed, complexity-bounded ASTs compiled into
parameterized queries. No raw SQL, identifier, sort expression, or JSON path from
a request is concatenated into SQL. Administrative rule explanation redacts data
the inspecting actor cannot access.

## Web security

- Define cookie flags, session rotation, CSRF protection, CORS defaults, origin
  checks, and SSE proxy behavior explicitly.
- Trust forwarded headers only from configured trusted proxies; never from the
  mere presence of `X-Forwarded-*`.
- Apply security headers without breaking required dashboard behavior. Avoid
  inline script so a restrictive Content Security Policy remains practical.
- Rate-limit authentication, recovery, token, expensive query, upload, and admin
  operations with privacy-preserving errors.

## Files and external calls

Normalize names, generate storage keys, prevent traversal/symlink attacks, bound
size and decompression, set safe content disposition/types, and separate staged
from committed files. Object URLs do not replace authorization unless explicitly
short-lived and signed.

Webhook/function targets use allow-listed schemes and destination policy. Protect
against DNS rebinding, link-local/cloud metadata access, redirect escalation, and
credential forwarding. Sign outgoing webhooks and constrain callback grants.

## Secrets and diagnostics

Secrets come from supported environment/config/secret-provider references and
are redacted structurally, not through best-effort string replacement. Logs,
errors, audit events, backup manifests, job payloads, admin APIs, and support
bundles must not reveal credentials or hidden record fields.

## Data integrity and recovery

Enable SQLite foreign keys and validate deployment filesystem assumptions.
Migrations are versioned, backed up, restart-safe, and refuse unknown future
versions. Backups use a SQLite-consistent mechanism and include an explicit file
storage strategy. A backup is not trusted until restore tests pass.

Audit records are append-oriented and capture actor, action, target, outcome,
time, and correlation ID without copying secrets. Tamper evidence and remote
audit export are later hardening options, not implied by an append-only UI.

## Required adversarial coverage

Maintain tests for auth enumeration, session fixation/revocation, CSRF/CORS,
scope confusion, rule bypass across every read path, SQL/expression injection,
path traversal, malicious uploads, replay gaps, slow consumers, job duplication,
lease theft, SSRF, callback escalation, secret leakage, proxy spoofing, migration
failure, backup/restore, and resource exhaustion. Fuzz parsers and decoders; run
Go race tests and bounded load tests before a stable release.

## Reporting and supported versions

Report suspected vulnerabilities privately to `security@trestle.dev`. Do not
open a public issue until maintainers have coordinated disclosure. Include the
affected version, deployment shape, reproduction, impact, and logs after
removing secrets and personal data.

Only the newest release line receives security fixes before the first stable
release. A supported-version window will be frozen at the stable release gate.
