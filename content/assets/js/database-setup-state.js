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
  return { computeState, authGateCopy, restartNotice, connectionState };
})();