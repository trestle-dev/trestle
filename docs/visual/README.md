# Trestle rendered-dashboard visual evidence

Deterministic screenshots captured with headless Chromium (the installed
`/snap/chromium/.../chrome` binary, classic `--headless`) against a real
running Trestle instance. Each capture uses a dedicated temporary profile;
only that process tree is launched and cleaned up. No user-owned Chromium
process is touched.

The model that produced these captures cannot render images, so in-model
visual inspection is not possible: the images are provided as artifacts for
human review, and the viewport/scenario/expected state is recorded below.

| File | Viewport | Scenario | Expected state |
| --- | --- | --- | --- |
| `first-run-desktop.png` | 1280x800 | Fresh Trestle instance, no administrator, dashboard loaded | First-run auth gate: database selection (SQLite selected, PostgreSQL available) and the "Create the administrator account" form, dark theme, no page-level overflow |
| `first-run-mobile.png` | 390x844 | Same fresh instance at a narrow viewport | Same first-run auth gate usable at a mobile width with no page-level horizontal overflow |

## Scope and limitation

Browser captures cover the first-run dashboard states reachable without an
authenticated session. The authenticated status-card degraded states (database
unavailable, starting, session expired) are exercised end to end by the
connection-recovery drill and the Node state-machine and dashboard-copy
assertions, but are not browser-screenshotted here; capturing them requires
an interactive authenticated session, which the plain headless-screenshot
harness cannot drive. That remains a documented browser-acceptance
limitation, not visual proof.