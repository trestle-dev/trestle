// CP12R5 owned-process-group cleanup regression.
//
// Drives the real signalGroup() helper (scripts/browser-cleanup.mjs) against
// real detached child processes and proves the cleanup routine is bounded and
// never hangs on a missed exit event:
//
//   1. Chromium still running during cleanup: SIGTERM kills the owned group.
//   2. Chromium already exited before cleanup: detected via exitCode, resolves
//      immediately (no await of a never-firing exit event).
//   3. Chromium exits between the state check and signal delivery: bounded.
//   4. Chromium ignores SIGTERM: the SIGTERM wait times out and SIGKILL on the
//      same owned group escalates within its own bound.
//   5. An unrelated sentinel process survives the owned-group kill in every
//      case above is exercised by the browser harness; here case 5 re-checks
//      that killing one owned group never reaches a sibling group.
import {spawn} from "node:child_process";
import {signalGroup, alreadyExited} from "./browser-cleanup.mjs";

const failures = [];
const check = (label, cond, detail) => {
  if (!cond) {
    failures.push(`${label}: ${detail}`);
  }
};

const spawnOwned = (code = "setInterval(()=>{}, 1<<30)") =>
  spawn(process.execPath, ["-e", code], {stdio: "ignore", detached: true});
const alive = (pid) => {
  try { process.kill(pid, 0); return true; } catch { return false; }
};
const stop = (p) => { try { process.kill(-p.pid, "SIGKILL"); } catch {} };

// 1. Running during cleanup: SIGTERM terminates the owned group.
{
  const p = spawnOwned();
  await new Promise((r) => setTimeout(r, 150));
  check("1: child is running", alive(p.pid), "child not alive");
  const t0 = Date.now();
  const ok = await signalGroup(p, "SIGTERM", 5000);
  check("1: SIGTERM terminated the owned group", ok === true, `ok=${ok}`);
  check("1: terminated within bound", Date.now() - t0 < 5000, `elapsed=${Date.now() - t0}`);
}

// 2. Already exited before cleanup: must not hang waiting for an exit event
// that already fired.
{
  const p = spawnOwned("process.exit(0)");
  await new Promise((r) => setTimeout(r, 300));
  check("2: alreadyExited detected", alreadyExited(p), `exitCode=${p.exitCode} signalCode=${p.signalCode}`);
  const t0 = Date.now();
  const ok = await signalGroup(p, "SIGTERM", 5000);
  check("2: pre-exited resolves immediately", ok === true && Date.now() - t0 < 2000, `ok=${ok} elapsed=${Date.now() - t0}`);
}

// 3. Exits between the initial state check and signal delivery: bounded, no hang.
{
  const p = spawnOwned("setTimeout(()=>process.exit(0), 60)");
  const t0 = Date.now();
  const ok = await signalGroup(p, "SIGTERM", 5000);
  check("3: mid-flight exit resolved", ok === true, `ok=${ok}`);
  check("3: bounded", Date.now() - t0 < 5000, `elapsed=${Date.now() - t0}`);
}

// 4. Ignores SIGTERM: the SIGTERM wait times out, then SIGKILL escalates
// within its own bound. Both waits are bounded, so the whole routine finishes.
{
  const p = spawnOwned("process.on('SIGTERM',()=>{}); setInterval(()=>{}, 1<<30)");
  await new Promise((r) => setTimeout(r, 150));
  const t0 = Date.now();
  const term = await signalGroup(p, "SIGTERM", 150);
  const termElapsed = Date.now() - t0;
  check("4: SIGTERM-ignoring child times out", term === false, `term=${term}`);
  check("4: SIGTERM wait bounded", termElapsed < 1000, `termElapsed=${termElapsed}`);
  const t1 = Date.now();
  const kill = await signalGroup(p, "SIGKILL", 150);
  check("4: SIGKILL escalates on the owned group", kill === true, `kill=${kill}`);
  check("4: SIGKILL wait bounded", Date.now() - t1 < 1000, `elapsed=${Date.now() - t1}`);
  check("4: SIGKILL path terminated the child", !alive(p.pid), "child still alive");
}

// 5. Sentinel survives: killing one owned group never reaches a sibling group.
{
  const sentinel = spawnOwned();
  const victim = spawnOwned();
  await new Promise((r) => setTimeout(r, 150));
  const ok = await signalGroup(victim, "SIGTERM", 5000);
  check("5: victim terminated", ok === true, `ok=${ok}`);
  check("5: sentinel survives the victim cleanup", alive(sentinel.pid), "sentinel died");
  stop(sentinel);
}

if (failures.length) {
  console.error(`browser cleanup regression: ${failures.length} failure(s)\n` + failures.join("\n"));
  process.exit(1);
}
console.log("browser cleanup regression passed: bounded, never hangs, sentinel survives");