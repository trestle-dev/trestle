// Registration-policy frontend behavioural regression: the complete one-time
// activation-token lifecycle, driven through the real event handlers.
//
// The dashboard JS is loaded into a sandbox whose DOM stub captures
// addEventListener handlers and can dispatch them. The test drives the actual
// provision/approve/reissue/refresh/revoke/reject/base-URL/copy/dismiss
// handlers (with fetch stubs) and asserts that the one-time token panel is
// preserved across every destructive in-view action and cleared only by
// deliberate dismissal or navigation confirmation.
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
    focus() {}, reset() {}, setAttribute() {},
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
// named registry for querySelectorAll-driven lists (approve/reissue/revoke/reject).
const listEls = new Map();
function listFor(sel) {
  if (!listEls.has(sel)) listEls.set(sel, []);
  return listEls.get(sel);
}
const host = {
  querySelector: (sel) => elFor(sel),
  querySelectorAll: (sel) => listFor(sel),
};
const document = {
  createElement: (t) => makeEl(t),
  createTextNode: (t) => ({ textContent: String(t) }),
  querySelector: (sel) => elFor(sel),
  getElementById: (id) => elFor("#" + id),
  querySelectorAll: (sel) => listFor(sel),
  addEventListener() {},
  body: makeEl("body"),
};
const fetched = [];
const sandbox = {
  document,
  window: { addEventListener() {}, dispatchEvent() {} },
  location: { origin: "http://localhost" },
  localStorage: { store: {}, getItem(k) { return this.store[k] ?? null; }, setItem(k, v) { this.store[k] = String(v); }, removeItem(k) { delete this.store[k]; } },
  crypto: { randomUUID: () => "id-1" },
  fetch: async (url, opt = {}) => {
    const u = String(url);
    const method = (opt.method || "GET").toUpperCase();
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
  confirm: () => true,
  prompt: () => "",
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
  __uiList: listFor,
  __uiEl: elFor,
};
vm.createContext(sandbox);

// Drive the harness inside the same script so the app's top-level bindings are
// reachable. Await is handled by the async IIFE; the test harness reads the
// collected evidence from the sandbox.
const harness = `
;(async () => {
  const host = __uiHost;
  const failures = [];
  const assert = (cond, msg) => { if (!cond) failures.push(msg); };

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
  await new Promise((r) => setTimeout(r, 10));
  assert(host.querySelector(".provision-result").innerHTML.indexOf("TOKEN123") !== -1, "provision token must appear after submit");
  assert(pendingToken === true, "pendingToken must be set after showing a token");

  // Approve: dispatch the real approve button handler.
  approveBtn.dispatch("click");
  await new Promise((r) => setTimeout(r, 10));
  assert(host.querySelector(".provision-result").innerHTML.indexOf("TOKEN123") !== -1, "approve token must appear");

  // Reissue: dispatch the real reissue handler.
  reissueBtn.dispatch("click");
  await new Promise((r) => setTimeout(r, 10));
  assert(host.querySelector(".provision-result").innerHTML.indexOf("TOKEN123") !== -1, "reissue token must appear");

  // Refresh: an ordinary in-view action must NOT destroy the token panel.
  const before = host.querySelector(".provision-result").innerHTML;
  const refreshBtn = host.querySelector("[data-refresh-users]");
  refreshBtn.dispatch("click");
  await new Promise((r) => setTimeout(r, 20));
  assert(host.querySelector(".provision-result").innerHTML === before, "Refresh must preserve the one-time token panel");

  // Revoke an invitation and reject a request: also must preserve the panel.
  revokeBtn.dispatch("click");
  await new Promise((r) => setTimeout(r, 20));
  assert(host.querySelector(".provision-result").innerHTML === before, "Revoke must preserve the one-time token panel");
  rejectBtn.dispatch("click");
  await new Promise((r) => setTimeout(r, 20));
  assert(host.querySelector(".provision-result").innerHTML === before, "Reject must preserve the one-time token panel");

  // Save activation base URL: must preserve the panel.
  const buf = host.querySelector(".baseurl-form");
  buf.elements = { baseurl: { value: "https://app.example.com/register" } };
  buf.dispatch("submit", { preventDefault() {} });
  await new Promise((r) => setTimeout(r, 20));
  assert(host.querySelector(".provision-result").innerHTML === before, "Saving the base URL must preserve the one-time token panel");

  // Copy the token: must not clear the panel.
  const copyBtn = host.querySelector("[data-copy-inv]");
  if (copyBtn) copyBtn.dispatch("click");
  assert(host.querySelector(".provision-result").innerHTML === before, "Copy must preserve the one-time token panel");

  // Navigate away: with a pending token the route guard confirms, then the
  // panel is destroyed by the new view (deliberate). Simulate a route switch
  // that replaces the view via a fresh renderUsers.
  await selectDashboardRoute("overview", "Overview");
  await new Promise((r) => setTimeout(r, 10));
  // pendingToken is still true until dismissed; the confirm guard allows leaving.
  pendingToken = true;
  const dismissed = await (async () => { dismissToken(host); return pendingToken === false && host.querySelector(".provision-result").innerHTML === ""; })();
  assert(dismissed, "dismiss must clear the token and pending flag");

  // After dismissal, navigating away requires no confirm and leaves no token.
  await selectDashboardRoute("users", "Users");
  assert(host.querySelector(".provision-result").innerHTML === "", "no token may reappear after dismissal");

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
if (!/selectDashboardRoute\(route,title\)\{if\(pendingToken/.test(jsSource)) failures.push("navigation away must warn while a token is pending");

if (failures.length) {
  throw new Error("registration UI regression:\n" + failures.join("\n"));
}
console.log("registration UI regression: ok (complete one-time-token lifecycle)");