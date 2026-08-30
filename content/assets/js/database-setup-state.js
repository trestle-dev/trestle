// First-run database selection state machine (pure, no DOM access).
// The dashboard DOM layer maps computeState()'s result onto hidden/disabled
// attributes; the Node regression suite drives computeState() through every
// transition without a browser. State contracts:
//
//   mode        "first-run" | "sign-in"
//   selectable  whether the provider may still be chosen (false after a
//               connection is persisted or when startup fixed the provider)
//   provider    "sqlite" | "postgres"
//   url         the PostgreSQL URL field value
//
// Invariants covered by the regression suite:
//   - an empty PostgreSQL URL leaves the test button disabled;
//   - a non-empty URL enables it;
//   - the first-administrator form is never shown together with the
//     PostgreSQL configuration form;
//   - after the connection is persisted (selectable=false) the interface
//     returns to administrator creation, never to a sign-in form, because the
//     auth gate copy is single-sourced and mutually exclusive.
globalThis.TrestleDatabaseSetup = (() => {
  function computeState(state) {
    const firstRun = state.mode === "first-run";
    const pendingPostgres = firstRun && state.selectable && state.provider === "postgres";
    const urlNonEmpty = Boolean(state.url && state.url.trim());
    return {
      previewVisible: firstRun,
      postgresConfigVisible: pendingPostgres,
      applyVisible: pendingPostgres,
      applyEnabled: pendingPostgres && urlNonEmpty,
      adminFormVisible: !pendingPostgres,
      adminEmailRequired: !pendingPostgres,
      adminPasswordRequired: !pendingPostgres
    };
  }
  function authGateCopy(setupRequired) {
    return setupRequired
      ? {
          kicker: "First-run setup",
          title: "Create the administrator account",
          description:
            "This deployment has no administrator yet. The account you create grants access to every administration surface.",
          submitLabel: "Create administrator",
          autocomplete: "new-password"
        }
      : {
          kicker: "Secure administration",
          title: "Sign in to Trestle",
          description: "Use an administrator account for this deployment.",
          submitLabel: "Sign in",
          autocomplete: "current-password"
        };
  }
  function restartNotice(version) {
    return {
      heading: "Restart required",
      body: `PostgreSQL ${version} is configured. Stop and start Trestle, then reload this page to create the administrator account.`
    };
  }
  // connectionState maps the process/database readiness probe to operator
  // copy: a clear heading, a consequence, and a next action. States: connecting
  // (initial), ready, starting (not_ready), databaseUnavailable (degraded) and
  // unreachable (network/process). Recovery is the transition back to ready on
  // a later successful probe.
  function connectionState(phase, errorCode) {
    if (phase === "connecting") {
      return {
        heading: "Connecting to Trestle",
        description: "Checking process and database readiness.",
        stateLabel: "Checking",
        kind: "connecting",
        nextAction: "This is a startup check. It resolves on its own."
      };
    }
    if (phase === "ready") {
      return {
        heading: "Trestle is ready",
        description: "The process and database are accepting work.",
        stateLabel: "Ready",
        kind: "ready",
        nextAction: "Nothing to do."
      };
    }
    if (phase === "databaseUnavailable") {
      return {
        heading: "Database unavailable",
        description: "The process is up but the database connection failed.",
        stateLabel: "Unavailable",
        kind: "databaseUnavailable",
        nextAction: "Check the database and the server logs, then retry."
      };
    }
    if (phase === "starting") {
      return {
        heading: "Trestle is starting",
        description: "Startup has not finished yet.",
        stateLabel: "Starting",
        kind: "starting",
        nextAction: "Check back shortly."
      };
    }
    return {
      heading: "Trestle is unreachable",
      description: "The server did not respond to the readiness probe.",
      stateLabel: "Unreachable",
      kind: "unreachable",
      nextAction: "Check the process, network and logs, then retry."
    };
  }
  return { computeState, authGateCopy, restartNotice, connectionState, viewState, staleState };
})();

// viewState is a reusable degraded-state component for dashboard panes. It
// maps a state kind and optional detail to plain-language operator copy: what
// happened, the operational consequence, and a specific next action. Kinds:
// loading, empty, error (permission-denied / partial-failure / unavailable),
// retrying, dead, deletionPending, stale. It never includes credentials or
// internal secrets.
// staleState is the documented realtime staleness rule: stale means a missing
// transport heartbeat, not merely "no business events recently". The server
// emits an observable `heartbeat` SSE event every 15 seconds and the client
// refreshes its activity time on every heartbeat, so an otherwise idle stream
// never goes stale. A stream is stale when no heartbeat (or business event)
// has arrived within the 30-second window and the stream is not paused. It is
// pure so a clock-controlled regression can prove the inactivity/recovery
// transitions without a browser.
function staleState(lastActivity, now, paused) {
  if (paused) return false;
  return now - lastActivity > 30000;
}
function viewState(kind, opts) {
  opts = opts || {};
  switch (kind) {
    case "loading":
      return { kind, title: "Loading", message: "Loading this view.", consequence: "No data is shown yet.", nextAction: "This resolves on its own." };
    case "empty":
      return { kind, title: "Nothing here yet", message: opts.message || "This view has no data yet.", consequence: "Nothing to show.", nextAction: "Create the first item to see it here." };
    case "error":
      const code = opts.code || "";
      if (code === "permissionDenied" || code === "authorization_denied") {
        return { kind, title: "Permission denied", message: "You do not have permission to view or change this.", consequence: "The action was not applied.", nextAction: "Ask an administrator for the required access." };
      }
      if (code === "database_unavailable") {
        return { kind, title: "Database unavailable", message: "The database connection failed.", consequence: "This view cannot load data right now.", nextAction: "Check the database and server logs, then retry." };
      }
      return { kind, title: "Could not load this view", message: opts.message || "The request could not be completed.", consequence: "The requested operation did not finish.", nextAction: "Check server logs, then retry." };
    case "retrying":
      return { kind, title: "Job is retrying", message: opts.message || "The job failed and is scheduled to retry.", consequence: "It is not complete yet.", nextAction: "You can wait for the retry or cancel the job." };
    case "dead":
      return { kind, title: "Job failed permanently", message: opts.message || "The job exhausted its retries.", consequence: "It will not run again automatically.", nextAction: "Retry the job or investigate the last error." };
    case "deletionPending":
      return { kind, title: "File deletion is pending", message: "The file is marked for deletion and is unavailable.", consequence: "The stored object has not been removed yet.", nextAction: "Trestle retries automatically; you can also trigger cleanup." };
    case "stale":
      return { kind, title: "Realtime connection is stale", message: "The live event stream has not delivered recently.", consequence: "Events may be delayed.", nextAction: "Check the connection and re-open the stream." };
    default:
      return { kind: "error", title: "Could not load this view", message: "The request could not be completed.", consequence: "The requested operation did not finish.", nextAction: "Check server logs, then retry." };
  }
}