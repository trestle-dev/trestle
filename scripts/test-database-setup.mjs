// CP2 first-run database-selection state-machine regression test.
//
// Drives the pure TrestleDatabaseSetup state machine (content/assets/js/
// database-setup-state.js) through every first-run transition the checkpoint
// contract requires, without a browser. The DOM layer in database-setup.js and
// script.js maps the computed state onto hidden/disabled attributes; the
// browser-quality gate also asserts the structural guarantees.
import {readFile} from "node:fs/promises";
import vm from "node:vm";

const source = await readFile(
  new URL("../content/assets/js/database-setup-state.js", import.meta.url),
  "utf8"
);
vm.runInThisContext(source);
const machine = globalThis.TrestleDatabaseSetup;
if (!machine) {
  console.error("TrestleDatabaseSetup state machine was not defined");
  process.exit(1);
}

let failures = 0;
const check = (label, actual, expected) => {
  const a = JSON.stringify(actual);
  const e = JSON.stringify(expected);
  if (a !== e) {
    failures += 1;
    console.error(`- ${label}: got ${a}, want ${e}`);
  }
};

const firstRun = {mode: "first-run", selectable: true, provider: "sqlite", url: ""};
const postgresSelectedEmpty = {mode: "first-run", selectable: true, provider: "postgres", url: ""};
const postgresSelectedFilled = {mode: "first-run", selectable: true, provider: "postgres", url: "postgres://u@host/db?sslmode=require"};
const savedAwaitingRestart = {mode: "first-run", selectable: false, provider: "postgres", url: ""};
const signIn = {mode: "sign-in", selectable: false, provider: "postgres", url: ""};

// 1. Fresh first-run, SQLite recommended: preview and administrator form
// visible, PostgreSQL configuration hidden.
check("fresh sqlite", machine.computeState(firstRun), {
  previewVisible: true,
  postgresConfigVisible: false,
  applyVisible: false,
  applyEnabled: false,
  adminFormVisible: true,
  adminEmailRequired: true,
  adminPasswordRequired: true
});

// 2. PostgreSQL selected, empty URL: the test button is visible but disabled,
// the administrator form is hidden (never shown simultaneously).
const empty = machine.computeState(postgresSelectedEmpty);
check("postgres selected, empty URL", {
  postgresConfigVisible: empty.postgresConfigVisible,
  applyVisible: empty.applyVisible,
  applyEnabled: empty.applyEnabled,
  adminFormVisible: empty.adminFormVisible,
  adminEmailRequired: empty.adminEmailRequired,
  adminPasswordRequired: empty.adminPasswordRequired
}, {
  postgresConfigVisible: true,
  applyVisible: true,
  applyEnabled: false,
  adminFormVisible: false,
  adminEmailRequired: false,
  adminPasswordRequired: false
});

// 3. Non-empty valid URL enables the test button; hover styling is attached
// only to enabled buttons (checked separately in the CSS contract), so the
// enabled state must be the one carrying the interactive affordance.
const filled = machine.computeState(postgresSelectedFilled);
check("postgres selected, filled URL", {
  postgresConfigVisible: filled.postgresConfigVisible,
  applyVisible: filled.applyVisible,
  applyEnabled: filled.applyEnabled,
  adminFormVisible: filled.adminFormVisible
}, {
  postgresConfigVisible: true,
  applyVisible: true,
  applyEnabled: true,
  adminFormVisible: false
});

// 4. Successful connection test: the provider is no longer selectable, so the
// interface returns to administrator creation (restart is pending). The two
// forms are still never shown together.
const saved = machine.computeState(savedAwaitingRestart);
check("saved awaiting restart", {
  postgresConfigVisible: saved.postgresConfigVisible,
  applyVisible: saved.applyVisible,
  applyEnabled: saved.applyEnabled,
  adminFormVisible: saved.adminFormVisible
}, {
  postgresConfigVisible: false,
  applyVisible: false,
  applyEnabled: false,
  adminFormVisible: true
});

// 5. The restart notice explains the required restart explicitly.
const notice = machine.restartNotice("18.6 (Ubuntu 18.6-0ubuntu0.26.04.1)");
check("restart notice heading", notice.heading, "Restart required");
if (!/PostgreSQL 18\.6/.test(notice.body) || !/Stop and start Trestle/.test(notice.body)) {
  failures += 1;
  console.error("- restart notice body does not explain the restart step: " + notice.body);
}

// 6. After restart the interface says the user is creating the first
// administrator, not merely signing in, and never both.
const setupCopy = machine.authGateCopy(true);
const signInCopy = machine.authGateCopy(false);
check("first-run copy title", setupCopy.title, "Create the administrator account");
check("first-run copy submit", setupCopy.submitLabel, "Create administrator");
check("first-run copy autocomplete", setupCopy.autocomplete, "new-password");
check("sign-in copy title", signInCopy.title, "Sign in to Trestle");
check("sign-in copy submit", signInCopy.submitLabel, "Sign in");
check("sign-in copy autocomplete", signInCopy.autocomplete, "current-password");
if (setupCopy.title === signInCopy.title) {
  failures += 1;
  console.error("- first-run and sign-in copy must never be identical");
}

// 7. Sign-in mode (not first-run): the database preview is hidden entirely and
// the administrator form is shown.
check("sign-in mode", machine.computeState(signIn), {
  previewVisible: false,
  postgresConfigVisible: false,
  applyVisible: false,
  applyEnabled: false,
  adminFormVisible: true,
  adminEmailRequired: true,
  adminPasswordRequired: true
});

if (failures) {
  console.error(`database setup state machine: ${failures} failure(s)`);
  process.exit(1);
}
console.log("database setup state machine: all transitions pass");