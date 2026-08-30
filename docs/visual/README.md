# Trestle rendered-dashboard visual evidence

Captures are produced by `scripts/browser-check.mjs`, a reproducible harness
that launches a disposable Trestle instance, seeds deterministic state, drives
the real SPA in headless Chromium over the DevTools Protocol, checks for
uncaught JavaScript errors and rejected promises, and records screenshots.
It launches Chromium with a dedicated temporary profile, records the owned PID
and terminates only that process tree (no name-wide or user-owned cleanup).

The model that produced these captures cannot render images, so in-model visual
inspection is not possible: the images are artifacts for human review, with the
viewport/scenario/expected state recorded below.

| File | Viewport | Scenario | Expected state |
| --- | --- | --- | --- |
| `first-run-desktop.png` | 1280x800 | Fresh instance, no administrator | First-run auth gate with database selection and administrator creation |
| `first-run-mobile.png` | 390x844 | Same fresh instance | Same gate usable at a mobile width, no horizontal overflow |
| `jobs-degraded-desktop.png` | 1280x800 | Administrator signed in, Jobs view with seeded succeeded/dead/retrying jobs | Jobs list renders the statuses (including "Job failed permanently" and "Job is retrying") with no JavaScript error |
| `jobs-degraded-mobile.png` | 390x844 | Same Jobs view | Same list usable at a mobile width |
| `session-expired-desktop.png` | 1280x800 | Administrator session revoked mid-use, an admin request then 401s | The SPA returns to the auth gate with a "Session expired" heading and sign-in next action; the rejected view is hidden behind it |

The browser harness fails the run on any uncaught JavaScript error or rejected
promise observed during these flows.

## Scope and limitation

Captured states are the first-run gate and the authenticated Jobs degraded
states and session-expired flow. Other requested degraded states (database
unavailable, permission denied on a specific view, backup progress, realtime
stale) are exercised by the connection-recovery drill and the Node
state-machine/dashboard-copy assertions but are not individually
browser-screenshotted; that remains a documented browser-acceptance
limitation, not visual proof.