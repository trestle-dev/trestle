# Trestle rendered-dashboard visual evidence

Captures are produced by `scripts/browser-check.mjs`, a reproducible harness
that launches a disposable Trestle instance, seeds deterministic state, drives
the real SPA in headless Chromium over the DevTools Protocol, checks for
uncaught JavaScript errors and rejected promises, and records screenshots.
It launches Chromium detached in its own process group with a dedicated
temporary profile, records the owned process-group ID, and terminates only that
group (SIGTERM then, on timeout, SIGKILL); no name-wide or user-owned cleanup,
and an unrelated sentinel process is verified to survive the cleanup.

The model that produced these captures cannot render images, so in-model visual
inspection is not possible: the images are artifacts for human review, with the
viewport/scenario/expected state recorded below.

| File | Viewport | Scenario | Expected state |
| --- | --- | --- | --- |
| `first-run-desktop.png` | 1280x800 | Fresh instance, no administrator | First-run auth gate with database selection and administrator creation |
| `first-run-mobile.png` | 390x844 | Same fresh instance | Same gate usable at a mobile width, no horizontal overflow |
| `jobs-degraded-desktop.png` | 1280x800 | Administrator signed in, Jobs view with seeded succeeded/dead/retrying jobs | Jobs list renders the statuses (including "Job failed permanently" and "Job is retrying") with no JavaScript error |
| `jobs-degraded-mobile.png` | 390x844 | Same Jobs view | Same list usable at a mobile width |
| `realtime-healthy-heartbeat-desktop.png` | 1280x800 | Administrator signed in, Realtime view open with the server heartbeat stream flowing | Connection state reports Connected; the real 15-second SSE heartbeat keeps an idle stream live |
| `realtime-stale-heartbeat-loss-desktop.png` | 1280x800 | Realtime view with the client heartbeat gap simulated via the controller hook (activity pinned past the 30s window) | Connection state reports Stale ("no heartbeat for a while"); the real staleness interval drives the transition |
| `realtime-recovered-desktop.png` | 1280x800 | A delivered heartbeat restores activity (the real recovery path) | Connection state returns to Connected ("live") |
| `session-expired-desktop.png` | 1280x800 | Administrator session revoked mid-use, an admin request then 401s | The SPA returns to the auth gate with a "Session expired" heading and sign-in next action; the rejected view is hidden behind it |

The browser harness fails the run on any uncaught JavaScript error or rejected
promise observed during these flows.

## Scope and limitation

Captured states are the first-run gate, the authenticated Jobs degraded
states, the Realtime healthy/stale/recovered transitions, and the
session-expired flow. Other requested degraded states (database unavailable,
backup progress, pending file deletion) are exercised by the connection-
recovery/restore drills and the Node state-machine/dashboard-copy assertions
but are not individually browser-screenshotted; that remains a documented
browser-acceptance limitation, not visual proof. The realtime stale/recovered
transitions are driven in the real SPA with the heartbeat gap simulated through
the exposed controller test hook rather than server fault injection.