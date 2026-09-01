// Registration-policy frontend behavioural regression: the complete one-time
// activation-token lifecycle and the guarded navigation state machine, driven
// through the real event handlers.
//
// The dashboard JS is loaded into a sandbox whose DOM stub captures
// addEventListener handlers and can dispatch them. The test drives the actual
// provision/approve/reissue/refresh/revoke/reject/base-URL/copy/dismiss
// handlers and the single authoritative route navigation (selectDashboardRoute
// / openCollectionRoute / restoreDashboardRoute / logout), with controllable
// confirm responses, history, popstate and per-URL fetch failures.
//
// Run with: node scripts/test-registration-ui.mjs [internal/web/public]

import { readFile } from "node:fs/promises";
import path from "node:path";
import process from "node:process";
import vm from "node:vm";
import { TextDecoder, TextEncoder } from "node:util";

const root = path.resolve(process.argv[2] || "internal/web/public");
const jsSource = await readFile(path.join(root, "assets/js/script.js"), "utf8");
const htmlSource = await readFile(path.join(root, "index.html"), "utf8");

const confirmState = { value: true, calls: 0 };
const failUrls = new Set();
const historyCalls = [];
const assignCalls = [];

function makeEl(tag = "div") {
  const listeners = new Map();
  return {
    tagName: (tag || "div").toUpperCase(),
    children: [],
    dataset: {},
    style: {},
    classList: { add() {}, remove() {}, toggle() {}, contains() { return false; } },
    _text: "",
    _innerHTML: "",
    hidden: false,
    disabled: false,
    value: "",
    checked: false,
    placeholder: "",
    type: "submit",
    href: "",
    scrollTop: 0,
    scrollHeight: 0,
    elements: {},
    addEventListener(type, fn) { const k = type || "click"; const a = listeners.get(k) || []; a.push(fn); listeners.set(k, a); },
    dispatch(type, event = {}) { const a = listeners.get(type || "click") || []; for (const fn of a) fn(event); },
    click() { this.dispatch("click", { preventDefault() {} }); },
    focus() {}, reset() {}, setAttribute() {}, removeAttribute() {},
    getAttribute(name) { return name === "href" ? this.href : null; },
    append(...x) { for (const k of x) if (k != null) this.children.push(k); },
    appendChild(x) { this.children.push(x); return x; },
    remove() {},
    set textContent(v) { this._text = String(v); },
    get textContent() { return this._text; },
    set innerHTML(v) { this._innerHTML = String(v); this.children = []; },
    get innerHTML() { return this._innerHTML; },
    querySelector(sel) { return elFor(sel); },
    querySelectorAll(sel) { return listFor(sel); },
  };
}

const els = new Map();
function elFor(sel) {
  if (!els.has(sel)) els.set(sel, makeEl());
  return els.get(sel);
}
const listEls = new Map();
function listFor(sel) {
  if (!listEls.has(sel)) listEls.set(sel, []);
  return listEls.get(sel);
}

// Pre-seed the [data-route] navigation links so the single authoritative
// route-listener attaches to elements the test can click.
for (const r of ["overview", "collections", "users", "settings", "files", "realtime", "audit", "jobs", "integrations", "api", "backups"]) {
  const link = makeEl("a");
  link.dataset.route = r;
  link.textContent = r;
  link.href = "/" + r;
  listFor("[data-route]").push(link);
}

const host = {
  querySelector: (sel) => elFor(sel),
  querySelectorAll: (sel) => listFor(sel),
};

// queryRouteLink resolves `[data-route="name"]` selectors to the real link.
function queryRouteLink(sel) {
  const m = /\[data-route=["']?([^"'\]]+)["']?\]/.exec(sel);
  if (m) {
    const found = listFor("[data-route]").find((el) => el.dataset.route === m[1]);
    if (found) return found;
  }
  return elFor(sel);
}

const windowListeners = new Map();
function windowOn(type, fn) {
  const a = windowListeners.get(type) || [];
  a.push(fn);
  windowListeners.set(type, a);
}
function windowEmit(type) {
  for (const fn of windowListeners.get(type) || []) fn();
}

const document = {
  createElement: (t) => makeEl(t),
  createTextNode: (t) => ({ textContent: String(t) }),
  querySelector: (sel) => queryRouteLink(sel),
  getElementById: (id) => elFor("#" + id),
  querySelectorAll: (sel) => listFor(sel),
  addEventListener() {},
  body: makeEl("body"),
};
const fetched = [];
const locationObj = { origin: "http://localhost", pathname: "/", assign(url) { assignCalls.push(String(url)); } };
const sandbox = {
  document,
  window: { addEventListener: windowOn, dispatchEvent: (e) => windowEmit(e.type), location: locationObj },
  location: locationObj,
  history: { replaceState(_s, _t, url) { historyCalls.push(String(url)); } },
  localStorage: { store: {}, getItem(k) { return this.store[k] ?? null; }, setItem(k, v) { this.store[k] = String(v); }, removeItem(k) { delete this.store[k]; } },
  crypto: { randomUUID: () => "id-1" },
  fetch: async (url, opt = {}) => {
    const u = String(url);
    const method = (opt.method || "GET").toUpperCase();
    if (failUrls.has(u) || failUrls.has(method + " " + u)) throw new Error("network failure");
    fetched.push({ url: u, method, body: opt.body || "" });
    let body;
    if (u === "/admin/v1/app-registration/invitations" && method === "POST") body = { id: "inv_1", kind: "activate", email: "x@example.com", expiresAt: "2027-01-01T00:00:00Z", token: "TOKEN123" };
    else if (u.includes("/approve") || u.includes("/reissue")) body = { id: "inv_1", kind: "activate", email: "x@example.com", expiresAt: "2027-01-01T00:00:00Z", token: "TOKEN123" };
    else if (u === "/admin/v1/app-registration/activation-base-url" && method === "PUT") body = { ok: true };
    else if (u === "/admin/v1/app-registration/invitations" && method === "GET") body = { items: [{ id: "inv_1", kind: "activate", email: "x@example.com", createdAt: "2026-01-01T00:00:00Z", expiresAt: "2027-01-01T00:00:00Z" }] };
    else if (u === "/admin/v1/app-registration/requests" && method === "GET") body = { items: [{ id: "req_1", email: "x@example.com", status: "pending", createdAt: "2026-01-01T00:00:00Z" }] };
    else body = { items: [], policy: "closed", setAt: "2026-01-01T00:00:00Z", activationBaseUrl: "" };
    return {
      ok: true, status: method === "POST" && u.includes("/invitations") ? 201 : 200,
      headers: { get: (h) => (String(h).toLowerCase() === "content-type" ? "application/json" : "") },
      json: async () => body,
      text: async () => JSON.stringify(body),
    };
  },
  confirm: () => { confirmState.calls++; return confirmState.value; },
  prompt: () => "",
  CSS: { escape: (s) => s },
  TrestleDatabaseSetup: {
    connectionState: (k) => ({ kind: k, heading: k, description: k, stateLabel: k, nextAction: k }),
    viewState: (k) => ({ title: k }),
    restartNotice: () => ({ heading: "", body: "" }),
  },
  FileReader: class {}, Image: class {},
  requestAnimationFrame: () => 0,
  MutationObserver: class { observe() {} },
  AbortController,
  Event: class Event { constructor(type) { this.type = type; } },
  TextDecoder, TextEncoder,
  setTimeout, clearTimeout, setInterval, clearInterval,
  console,
  navigator: { clipboard: { writeText: async () => {} } },
  __uiHost: host,
  __uiFetched: fetched,
  __uiList: listFor,
  __uiEl: elFor,
  __uiSetConfirm: (v) => { confirmState.value = v; },
  __uiConfirmCalls: () => confirmState.calls,
  __uiHistoryCalls: () => historyCalls.slice(),
  __uiAssignCalls: () => assignCalls.slice(),
  __uiFailUrl: (u, on) => { if (on) failUrls.add(u); else failUrls.delete(u); },
  __uiWindowEmit: windowEmit,
};
vm.createContext(sandbox);

const harness = `
;(async () => {
  const host = __uiHost;
  const failures = [];
  const assert = (cond, msg) => { if (!cond) failures.push(msg); };
  const settle = () => new Promise((r) => setTimeout(r, 20));
  const mintCalls = () => __uiFetched.filter((f) => f.method === "POST" && f.url.includes("/invitations") || f.url.includes("/approve") || f.url.includes("/reissue")).length;
  const panel = () => host.querySelector(".provision-result").innerHTML;
  const view = () => document.getElementById("view-content").innerHTML;

  // Wrap the authoritative renderers so we can count exactly-once renders.
  const renderCalls = { users: 0, collections: 0, overview: 0 };
  {
    const u = dashboardRenderers.users;
    dashboardRenderers.users = async (...a) => { renderCalls.users++; try { return await u(...a); } catch (e) { return; } };
    const c = dashboardRenderers.collections;
    dashboardRenderers.collections = async (...a) => { renderCalls.collections++; try { return await c(...a); } catch (e) { return; } };
    const o = dashboardRenderers.overview;
    dashboardRenderers.overview = (...a) => { renderCalls.overview++; try { return o(...a); } catch (e) { return; } };
  }

  // Pre-seed named form elements the real loadRegistration touches during
  // render, so it does not throw before attaching the provision handler.
  __uiEl(".policy-form").elements = { policy: { value: "closed" } };
  __uiEl(".baseurl-form").elements = { baseurl: { value: "" } };
  __uiEl(".provision-form").elements = { email: { value: "person@example.com" } };

  // Pre-seed the rendered-list buttons so the real renderers attach their
  // handlers to registry elements we can later dispatch.
  const approveBtn = __uiEl("approve-btn"); approveBtn.dataset.approve = "req_1"; __uiList("[data-approve]").push(approveBtn);
  const rejectBtn = __uiEl("reject-btn"); rejectBtn.dataset.reject = "req_1"; __uiList("[data-reject]").push(rejectBtn);
  const revokeBtn = __uiEl("revoke-btn"); revokeBtn.dataset.revokeInv = "inv_1"; __uiList("[data-revoke-inv]").push(revokeBtn);
  const reissueBtn = __uiEl("reissue-btn"); reissueBtn.dataset.reissue = "req_1"; __uiList("[data-reissue]").push(reissueBtn);

  // Render the registration view and let loadRegistration populate it.
  await renderUsers();

  // Provision form: set the email input and submit via the real handler.
  const provForm = host.querySelector(".provision-form");
  provForm.elements = { email: { value: "person@example.com" } };
  provForm.dispatch("submit", { preventDefault() {} });
  await settle();
  assert(panel().indexOf("TOKEN123") !== -1, "provision token must appear after submit");
  assert(pendingToken === true, "pendingToken must be set after showing a token");

  // While a token is pending, minting actions are blocked and must not
  // overwrite the visible token.
  const mintAfterProvision = mintCalls();
  approveBtn.dispatch("click");
  await settle();
  assert(mintCalls() === mintAfterProvision, "approve must be blocked while a token is pending");
  assert(panel().indexOf("TOKEN123") !== -1, "blocked approve must not overwrite the visible token");
  reissueBtn.dispatch("click");
  await settle();
  assert(mintCalls() === mintAfterProvision, "reissue must be blocked while a token is pending");
  assert(panel().indexOf("TOKEN123") !== -1, "blocked reissue must not overwrite the visible token");

  // Explicit dismissal permits approve, which mints its own token.
  dismissToken(host);
  assert(pendingToken === false && panel() === "", "dismiss must clear the token and flag");
  approveBtn.dispatch("click");
  await settle();
  assert(panel().indexOf("TOKEN123") !== -1, "approve token must appear after dismissal");
  const mintAfterApprove = mintCalls();
  reissueBtn.dispatch("click");
  await settle();
  assert(mintCalls() === mintAfterApprove, "reissue must be blocked while the approve token is pending");
  dismissToken(host);
  reissueBtn.dispatch("click");
  await settle();
  assert(panel().indexOf("TOKEN123") !== -1, "reissue token must appear after dismissal");

  // Refresh / revoke / reject / base-URL save: ordinary in-view actions must
  // NOT destroy the token panel.
  const before = panel();
  const refreshBtn = host.querySelector("[data-refresh-users]");
  refreshBtn.dispatch("click");
  await settle();
  assert(panel() === before, "Refresh must preserve the one-time token panel");
  revokeBtn.dispatch("click");
  await settle();
  assert(panel() === before, "Revoke must preserve the one-time token panel");
  rejectBtn.dispatch("click");
  await settle();
  assert(panel() === before, "Reject must preserve the one-time token panel");
  const buf = host.querySelector(".baseurl-form");
  buf.elements = { baseurl: { value: "https://app.example.com/register" } };
  buf.dispatch("submit", { preventDefault() {} });
  await settle();
  assert(panel() === before, "Saving the base URL must preserve the one-time token panel");

  // Copy the token must not clear the panel.
  const copyBtn = host.querySelector("[data-copy-inv]");
  if (copyBtn) copyBtn.dispatch("click");
  assert(panel() === before, "Copy must preserve the one-time token panel");

  // ---- Navigation state machine -------------------------------------------

  // (a) Rejected confirmation: NOTHING happens. No render, no history, no
  // active-route change, no token-state change, no view change.
  __uiSetConfirm(false);
  const histBefore = __uiHistoryCalls().length;
  const titleBefore = document.getElementById("view-title").textContent;
  const viewBefore = view();
  selectDashboardRoute("users", "Users", "/users");
  await settle();
  assert(renderCalls.users === 0, "rejected navigation must not render the destination");
  assert(__uiHistoryCalls().length === histBefore, "rejected navigation must not update history");
  assert(document.getElementById("view-title").textContent === titleBefore, "rejected navigation must not change the active route");
  assert(view() === viewBefore, "rejected navigation must leave the current view in place");
  assert(panel() === before, "rejected navigation must keep the token panel");
  assert(pendingToken === true, "rejected navigation must keep pendingToken");

  // (b) Accepted confirmation: token and flag clear, history changes once,
  // destination renders once, active route changes.
  __uiSetConfirm(true);
  const confirmAt = __uiConfirmCalls();
  selectDashboardRoute("users", "Users", "/users");
  await settle();
  assert(pendingToken === false, "accepted navigation must clear pendingToken");
  assert(panel() === "", "accepted navigation must clear the token panel before rendering");
  assert(renderCalls.users === 1, "accepted navigation must render the destination exactly once");
  assert(__uiHistoryCalls().length === histBefore + 1, "accepted navigation must update history exactly once");
  assert(document.getElementById("view-title").textContent === "Users", "accepted navigation must update the active route");
  assert(__uiConfirmCalls() === confirmAt + 1, "accepted navigation must ask exactly once");

  // (c) Later navigation with no pending token: no phantom warning.
  selectDashboardRoute("collections", "Collections", "/collections");
  await settle();
  assert(renderCalls.collections === 1, "later navigation must render once");
  assert(pendingToken === false, "no phantom token state after navigation");
  assert(panel() === "", "no phantom token panel after navigation");

  // (d) Browser back/forward follows the same guarded contract.
  showToken(host, "Back-forward token", "BFTOKEN");
  assert(pendingToken === true, "token must be pending before back/forward");
  __uiSetConfirm(false);
  const histBF = __uiHistoryCalls().length;
  __uiWindowEmit("popstate");
  await settle();
  assert(pendingToken === true && panel().indexOf("BFTOKEN") !== -1, "rejected back/forward must keep the token");
  __uiSetConfirm(true);
  __uiWindowEmit("popstate");
  await settle();
  assert(pendingToken === false, "accepted back/forward must clear the token");
  assert(renderCalls.overview >= 1, "back/forward must render through the guarded path");

  // (e) openCollectionRoute cannot bypass the guard.
  showToken(host, "Collection token", "COLTOKEN");
  __uiSetConfirm(false);
  const histOC = __uiHistoryCalls().length;
  const colsBefore = renderCalls.collections;
  openCollectionRoute("issues", true);
  await settle();
  assert(__uiHistoryCalls().length === histOC, "openCollectionRoute must not bypass the rejected guard");
  assert(renderCalls.collections === colsBefore, "openCollectionRoute must not render on rejected guard");
  assert(pendingToken === true && panel().indexOf("COLTOKEN") !== -1, "openCollectionRoute rejected must keep the token");
  __uiSetConfirm(true);
  openCollectionRoute("issues", true);
  await settle();
  assert(pendingToken === false, "openCollectionRoute accepted must clear the token");
  assert(renderCalls.collections === colsBefore + 1, "openCollectionRoute accepted must render exactly once");
  assert(__uiHistoryCalls().length === histOC + 1, "openCollectionRoute accepted must update history once");

  // (f) Failed logout retains token protection.
  showToken(host, "Logout token", "LOGTOKEN");
  __uiFailUrl("DELETE /admin/v1/session", true);
  document.getElementById("logout").dispatch("click");
  await settle();
  __uiFailUrl("DELETE /admin/v1/session", false);
  assert(pendingToken === true && panel().indexOf("LOGTOKEN") !== -1, "failed logout must retain token protection");

  // (g) Successful logout clears it and leaves.
  document.getElementById("logout").dispatch("click");
  await settle();
  assert(pendingToken === false, "successful logout must clear pendingToken");
  assert(panel() === "", "successful logout must clear the token panel");
  assert(__uiAssignCalls().length === 1, "successful logout must navigate away");

  // (h) A second provision/approve/reissue cannot overwrite an undisposed token.
  __uiEl(".provision-form").elements = { email: { value: "person@example.com" } };
  provForm.dispatch("submit", { preventDefault() {} });
  await settle();
  assert(panel().indexOf("TOKEN123") !== -1 && pendingToken === true, "provision must show the first token");
  const mintBefore = mintCalls();
  provForm.dispatch("submit", { preventDefault() {} });
  await settle();
  assert(mintCalls() === mintBefore, "second provision must be blocked while a token is pending");
  assert(panel().indexOf("TOKEN123") !== -1, "blocked provision must not overwrite the visible token");
  assert(host.querySelector(".view-error").textContent.indexOf("Dismiss the current activation token") !== -1, "blocked mint must explain the dismissal requirement");

  // (i) Explicit dismissal permits a later token.
  dismissToken(host);
  assert(pendingToken === false && panel() === "", "dismiss must clear the token and flag");
  provForm.dispatch("submit", { preventDefault() {} });
  await settle();
  assert(panel().indexOf("TOKEN123") !== -1 && pendingToken === true, "after dismissal a later token may be issued");

  globalThis.__uiFailures = failures;
})();
`;
sandbox.__uiDone = vm.runInContext(jsSource + "\n" + harness, sandbox) || Promise.resolve();
await sandbox.__uiDone;

// Static HTML/JS contract checks.
const failures = sandbox.__uiFailures || [];
if (!jsSource.includes('<form class="policy-form">')) failures.push("policy form must be a real <form>");
if (!/policyForm\.addEventListener\(["']submit["'][^;]*?ev\.preventDefault\(\)/.test(jsSource)) failures.push("policy form must handle submit with preventDefault");
if (jsSource.indexOf('data-dismiss-token') === -1) failures.push("one-time panel must have an explicit Dismiss action");
if (jsSource.indexOf('function canIssueToken') === -1) failures.push("token-minting actions must be blocked while a token is pending");
if (!/function selectDashboardRoute\(route,title,href/.test(jsSource)) failures.push("route navigation must flow through one guarded function");
if (/data-route="users"\][^;]*addEventListener\("click",renderUsers/.test(jsSource)) failures.push("independent render listeners must be removed (single navigation path)");
if (jsSource.indexOf('pendingToken=false;const button=document.getElementById("logout")') !== -1) failures.push("logout must not clear pendingToken before the request succeeds");

if (failures.length) {
  throw new Error("registration UI regression:\n" + failures.join("\n"));
}
console.log("registration UI regression: ok (one-time-token lifecycle and guarded navigation)");