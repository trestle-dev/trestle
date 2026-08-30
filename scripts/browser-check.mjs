// Browser-visual and SPA regression harness (CP12R3/CP12R4).
//
// Launches a disposable Trestle instance, seeds deterministic job states,
// drives the real SPA in headless Chromium over the DevTools Protocol, checks
// for uncaught JavaScript errors and rejected promises, verifies the Jobs view
// renders retrying/dead/succeeded states, the Realtime view stays healthy on
// heartbeat and shows stale/recovered transitions, the session-expired flow,
// and captures desktop and mobile screenshots. Chromium is spawned detached in
// its own process group so cleanup can SIGTERM then SIGKILL only that owned
// group (PID/process-group ID recorded; no name-wide or user-owned cleanup), and
// an unrelated sentinel process is verified to survive the cleanup.
//
// Usage: node scripts/browser-check.mjs [--out DIR]
import {spawn, execSync} from "node:child_process";
import {mkdtemp, rm, writeFile, mkdir} from "node:fs/promises";
import {tmpdir} from "node:os";
import {join} from "node:path";

const root = new URL("../", import.meta.url).pathname;
const outDir = process.argv.includes("--out") ? process.argv[process.argv.indexOf("--out") + 1] : join(root, "docs", "visual");
const chromeCandidates = [
  "/snap/chromium/current/usr/lib/chromium-browser/chrome",
  "/usr/bin/chromium-browser",
  "/usr/bin/chromium",
];
const chrome = chromeCandidates.find((c) => {
  try { execSync(`test -x "${c}"`, {stdio: "ignore"}); return true; } catch { return false; }
});
if (!chrome) {
  console.error("no Chromium binary found; browser acceptance remains pending");
  process.exit(2);
}

const failures = [];
const jsErrors = [];
const work = await mkdtemp(join(tmpdir(), "trestle-browser."));
const profile = join(work, "profile");
await mkdir(profile);
const appPort = 29100 + (process.pid % 20000);
const cdpPort = appPort + 1;
const base = `http://127.0.0.1:${appPort}`;

const trestle = [];
let chromePid = null;
let chromeProc = null;
let sentinel = null;

const trestleBin = join(work, "trestle");
function startTrestle() {
  const p = spawn(trestleBin, ["--listen", `127.0.0.1:${appPort}`, "--data-dir", join(work, "data")], {stdio: "ignore"});
  trestle.push(p);
}
async function waitFor(url) {
  for (let i = 0; i < 100; i++) {
    try { const r = await fetch(url); if (r.ok) return; } catch {}
    await new Promise((r) => setTimeout(r, 100));
  }
  throw new Error(`timed out waiting for ${url}`);
}
function stopTrestle() {
  for (const p of trestle) { try { p.kill(); } catch {} }
}

try {
  // Build the application and the seed helper.
  execSync(`go build -o ${trestleBin} ./cmd/trestle`, {cwd: root, stdio: "inherit"});
  execSync(`go build -o ${join(work, "seed")} ./scripts/browser-seed`, {cwd: root, stdio: "inherit"});

  startTrestle();
  await waitFor(`${base}/system/health`);
  // Seed deterministic job states: stop, insert, restart.
  for (const p of trestle) { try { p.kill(); } catch {} }
  await new Promise((r) => setTimeout(r, 300));
  execSync(`${join(work, "seed")} ${join(work, "data", "trestle.db")}`, {stdio: "inherit"});
  startTrestle();
  await waitFor(`${base}/system/health`);

  // Launch Chromium with a dedicated profile and CDP, detached into its own
  // process group (PGID == chromePid) so cleanup can target only that group.
  chromeProc = spawn(chrome, [
    "--headless", "--no-sandbox", "--disable-gpu", "--no-first-run", "--no-default-browser-check",
    `--user-data-dir=${profile}`, `--remote-debugging-port=${cdpPort}`,
    "about:blank",
  ], {stdio: "ignore", detached: true});
  chromePid = chromeProc.pid;
  await waitFor(`http://127.0.0.1:${cdpPort}/json/version`);

  // Connect to the page target via CDP.
  const targets = await (await fetch(`http://127.0.0.1:${cdpPort}/json`)).json();
  const page = targets.find((t) => t.type === "page");
  const ws = new WebSocket(page.webSocketDebuggerUrl);
  let seq = 0;
  const pending = new Map();
  const send = (method, params = {}) => new Promise((resolve) => {
    const id = ++seq;
    pending.set(id, resolve);
    ws.send(JSON.stringify({id, method, params}));
  });
  ws.onmessage = (ev) => {
    const msg = JSON.parse(ev.data);
    if (msg.id && pending.has(msg.id)) { pending.get(msg.id)(msg); pending.delete(msg.id); }
    if (msg.method === "Runtime.exceptionThrown") {
      const d = msg.params.exceptionDetails?.exception?.description || "exception";
      jsErrors.push(d);
    }
    if (msg.method === "Runtime.consoleAPICalled" && msg.params.type === "error") {
      jsErrors.push(msg.params.args.map((a) => a.value ?? a.description ?? "").join(" "));
    }
  };
  await new Promise((res, rej) => { ws.onopen = res; ws.onerror = rej; });
  await send("Runtime.enable");
  await send("Page.enable");
  const evaluate = async (expr) => {
    const r = await send("Runtime.evaluate", {expression: expr, awaitPromise: true, returnByValue: true});
    if (r.result?.exceptionDetails) throw new Error(r.result.exceptionDetails.text);
    return r.result?.result?.value;
  };
  const screenshot = async (file) => {
    const shot = await send("Page.captureScreenshot", {format: "png"});
    await writeFile(file, Buffer.from(shot.result.data, "base64"));
  };
  const resize = async (w, h) => {
    await send("Emulation.setDeviceMetricsOverride", {width: w, height: h, deviceScaleFactor: 1, mobile: false});
  };

  // 1. First-run: submit the setup form through the real SPA.
  await send("Page.navigate", {url: base + "/"});
  await new Promise((r) => setTimeout(r, 1500));
  await evaluate(`(() => {
    document.querySelector("#auth-email").value = "admin@example.com";
    document.querySelector("#auth-password").value = "correct horse battery staple";
    document.querySelector("#auth-form").requestSubmit();
    return true;
  })()`);
  await new Promise((r) => setTimeout(r, 1500));
  await evaluate(`document.querySelector("#auth-submit").click(); true`);
  await new Promise((r) => setTimeout(r, 2000));

  // 2. Navigate to the Jobs view (real SPA nav) and verify retrying/dead/succeeded.
  await evaluate(`(() => { const l=[...document.querySelectorAll("[data-route]")].find(x=>x.textContent.trim()==="Jobs"||x.getAttribute("data-route")==="jobs"); if(l) l.click(); return true; })()`);
  await new Promise((r) => setTimeout(r, 2000));
  const jobsState = await evaluate(`(() => {
    const host = document.getElementById("view-content");
    if (!host) return "no-view";
    return {
      text: host.textContent.slice(0, 400),
      hasDead: host.textContent.includes("Job failed permanently"),
      hasRetry: host.textContent.includes("Job is retrying"),
      hasSucceeded: host.textContent.includes("succeeded"),
    };
  })()`);
  if (!jobsState.hasDead || !jobsState.hasRetry || !jobsState.hasSucceeded) {
    failures.push(`Jobs view did not render expected states: ${JSON.stringify(jobsState)}`);
  } else {
    await resize(1280, 800);
    await screenshot(join(outDir, "jobs-degraded-desktop.png"));
    await resize(390, 844);
    await screenshot(join(outDir, "jobs-degraded-mobile.png"));
  }

  // 3. Realtime: healthy via heartbeat, stale after heartbeat loss, recovered.
  // The state transitions run in the real SPA; the heartbeat gap is simulated
  // through the exposed controller hook (server fault injection is out of
  // scope for CP12), while the healthy capture rides the genuine server
  // heartbeat stream.
  const pollFor = async (expr, timeoutMs, stepMs = 250) => {
    const start = Date.now();
    while (Date.now() - start < timeoutMs) {
      if (await evaluate(expr)) return true;
      await new Promise((r) => setTimeout(r, stepMs));
    }
    return false;
  };
  await evaluate(`(() => { const l=[...document.querySelectorAll("[data-route]")].find(x=>x.getAttribute("data-route")==="realtime"); if(l) l.click(); return true; })()`);
  const realtimeConnected = await pollFor(`(() => { const e=document.querySelector("#view-content .connection-state"); return e ? e.textContent.startsWith("Connected") : false; })()`, 12000);
  if (!realtimeConnected) {
    failures.push("Realtime view did not reach Connected on heartbeat");
  } else {
    await resize(1280, 800);
    await screenshot(join(outDir, "realtime-healthy-heartbeat-desktop.png"));
    // Force a heartbeat gap: keep activity pinned to 0 past the 30s window so
    // the real staleness interval transitions the view to Stale.
    const stale = await pollFor(`(() => {
      window.__trestleRealtime.setActivity(0);
      const e=document.querySelector("#view-content .connection-state");
      return e ? e.textContent.startsWith("Stale") : false;
    })()`, 15000, 250);
    if (!stale) {
      failures.push("Realtime view did not enter Stale after heartbeat loss");
    } else {
      await screenshot(join(outDir, "realtime-stale-heartbeat-loss-desktop.png"));
      // A delivered heartbeat (via the real recovery path) restores Connected.
      await evaluate(`window.__trestleRealtime.mark(); true`);
      const recovered = await pollFor(`(() => { const e=document.querySelector("#view-content .connection-state"); return e ? e.textContent.startsWith("Connected") : false; })()`, 5000);
      if (!recovered) {
        failures.push("Realtime view did not recover after a heartbeat");
      } else {
        await screenshot(join(outDir, "realtime-recovered-desktop.png"));
      }
    }
  }
  await evaluate(`document.querySelector("[data-route=overview]")?.click(); true`);
  await new Promise((r) => setTimeout(r, 600));

  // 4. Session-expired flow: revoke the admin session, then force an admin
  // request so the SPA returns to the auth gate with "Session expired".
  await evaluate(`fetch("/admin/v1/session").then(r=>r.json()).then(s=>fetch("/admin/v1/session", {method:"DELETE", headers:{"X-Trestle-CSRF": s.csrfToken}})).catch(()=>{}); true`);
  await new Promise((r) => setTimeout(r, 500));
  const afterLogout = await evaluate(`fetch("/admin/v1/session").then(r=>r.json()).then(s=>s.authenticated)`);
  if (afterLogout !== false) {
    failures.push(`session still authenticated after logout: ${afterLogout}`);
  }
  await evaluate(`document.querySelector("[data-route=overview]")?.click(); true`);
  await new Promise((r) => setTimeout(r, 800));
  await evaluate(`(() => { const l=[...document.querySelectorAll("[data-route]")].find(x=>x.getAttribute("data-route")==="collections"); if(l) l.click(); return true; })()`);
  await new Promise((r) => setTimeout(r, 1500));
  const sessionState = await evaluate(`(() => {
    const gate = document.getElementById("auth-gate");
    const view = document.getElementById("view-content");
    const gateText = gate && !gate.classList.contains("hidden") ? gate.textContent : "gate-hidden";
    return {gate: gateText.slice(0, 80), viewHidden: view ? view.hidden : "no-view", viewText: view ? view.textContent.slice(0, 120) : ""};
  })()`);
  const gateText = (sessionState && sessionState.gate) || sessionState;
  if (!String(gateText).includes("Session expired")) {
    failures.push(`session-expired state not shown: ${JSON.stringify(sessionState)}`);
  } else {
    await resize(1280, 800);
    await screenshot(join(outDir, "session-expired-desktop.png"));
  }

  // Report uncaught JS errors.
  if (jsErrors.length) {
    failures.push(`uncaught JavaScript errors/rejected promises: ${jsErrors.slice(0, 5).join(" | ")}`);
  }

  // An unrelated sentinel process (spawned in the harness's own group, not
  // Chromium's) must survive the Chromium group cleanup.
  try {
    sentinel = spawn(process.execPath, ["-e", "setInterval(()=>{}, 1<<30)"], {stdio: "ignore"});
  } catch {}

  ws.close();
} catch (err) {
  failures.push(`harness error: ${err.message}`);
} finally {
  stopTrestle();
  // Terminate only the Chromium process group we own (detached spawn made the
  // PID the process-group ID). Close the CDP socket first, SIGTERM the owned
  // group, wait for exit, and escalate to SIGKILL for the same group only
  // after a timeout. Never any executable-name or user-owned cleanup.
  if (chromePid) {
    const exited = new Promise((res) => chromeProc.once("exit", res));
    try { process.kill(-chromePid, "SIGTERM"); } catch {}
    const done = await Promise.race([
      exited.then(() => true),
      new Promise((r) => setTimeout(() => r(false), 5000)),
    ]);
    if (!done) {
      try { process.kill(-chromePid, "SIGKILL"); } catch {}
      await exited;
    }
    await new Promise((r) => setTimeout(r, 400));
  }
  // Verify the sentinel survived the Chromium group cleanup, then release it.
  if (sentinel && sentinel.pid) {
    let survived = false;
    try { process.kill(sentinel.pid, 0); survived = true; } catch {}
    if (!survived) {
      failures.push("sentinel process died during Chromium process-group cleanup");
    }
    try { process.kill(sentinel.pid, "SIGKILL"); } catch {}
  }
  for (let attempt = 0; attempt < 5; attempt++) {
    try { await rm(work, {recursive: true, force: true}); break; } catch { await new Promise((r) => setTimeout(r, 500)); }
  }
}

if (failures.length) {
  console.error("browser check FAILED:\n" + failures.join("\n"));
  process.exit(1);
}
console.log("browser check passed: jobs degraded states, session-expired, no uncaught JS errors");