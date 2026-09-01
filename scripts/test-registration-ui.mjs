// Registration-policy frontend regression: one-time activation tokens must
// remain visible and copyable after a list refresh, and the policy form must
// not perform an unintended native submission.
//
// Loads content/assets/js/script.js (and its embedded index.html) into a
// minimal DOM sandbox and exercises showToken/refreshLists and the policy
// form submit wiring. The assertions run inside the same script execution so
// the application's top-level bindings are reachable.
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
    addEventListener() {},
    focus() {},
    reset() {},
    append(...x) { for (const k of x) if (k != null) this.children.push(k); },
    appendChild(x) { this.children.push(x); return x; },
    remove() {},
    set textContent(v) { this._text = String(v); },
    get textContent() { return this._text; },
    set innerHTML(v) { this._innerHTML = String(v); this.children = []; },
    get innerHTML() { return this._innerHTML; },
    querySelector() { return makeEl(); },
    querySelectorAll() { return []; },
  };
}

const els = new Map();
const elFor = (sel) => {
  if (!els.has(sel)) els.set(sel, makeEl());
  return els.get(sel);
};
const host = { querySelector: (sel) => elFor(sel), querySelectorAll: () => [] };
const document = {
  createElement: (t) => makeEl(t),
  createTextNode: (t) => ({ textContent: String(t) }),
  querySelector: (sel) => elFor(sel),
  getElementById: (id) => elFor("#" + id),
  querySelectorAll: () => [],
  addEventListener: () => {},
  body: makeEl("body"),
};
const sandbox = {
  document,
  window: { addEventListener() {}, dispatchEvent() {} },
  location: { origin: "http://localhost" },
  localStorage: { store: {}, getItem(k) { return this.store[k] ?? null; }, setItem(k, v) { this.store[k] = String(v); }, removeItem(k) { delete this.store[k]; } },
  crypto: { randomUUID: () => "id-1" },
  fetch: async () => ({
    ok: true, status: 200,
    headers: { get: (h) => (String(h).toLowerCase() === "content-type" ? "application/json" : "") },
    json: async () => ({ items: [], policy: "closed", setAt: "2026-01-01T00:00:00Z", activationBaseUrl: "" }),
    text: async () => "{}",
  }),
  confirm: () => true,
  prompt: () => "",
  FileReader: class {},
  Image: class {},
  requestAnimationFrame: () => 0,
  MutationObserver: class { observe() {} },
  AbortController,
  TextDecoder,
  TextEncoder,
  setTimeout, clearTimeout, setInterval, clearInterval,
  console,
  navigator: { clipboard: { writeText: async () => {} } },
  __uiHost: host,
};
vm.createContext(sandbox);

// Assertions run inside the same script so the app's top-level bindings
// (activationBaseUrl, showToken, refreshLists) are reachable.
const harness = `
;(function(){
  const host = __uiHost;
  const failures = [];
  const assert = (cond, msg) => { if (!cond) failures.push(msg); };

  // showToken renders a copy-once panel; refreshLists must not clear it.
  showToken(host, "Activation token", "tok123");
  const panel = host.querySelector(".provision-result").innerHTML;
  assert(panel.indexOf("tok123") !== -1, "shown token must appear in the one-time panel");
  assert(panel.indexOf("Copy this now") !== -1, "one-time panel must warn it will not be shown again");
  assert(panel.indexOf('data-copy-inv="tok123"') !== -1 && panel.indexOf("Copy token") !== -1, "copy button must be present on the one-time panel");

  const before = host.querySelector(".provision-result").innerHTML;
  refreshLists(host);
  assert(host.querySelector(".provision-result").innerHTML === before, "refreshLists must not clear the one-time token panel");

  // Token-showing operations refresh lists without re-rendering the view.
  const src = __uiSource;
  let idx = 0;
  let checked = 0;
  while ((idx = src.indexOf("showToken(host,", idx)) !== -1) {
    if (src[idx + "showToken(host,".length] === '"') {
      const window = src.slice(idx, idx + 220);
      if (window.indexOf("await renderUsers()") !== -1) failures.push("a token-showing operation re-renders the whole view and destroys the one-time token");
      if (window.indexOf("await refreshLists(host)") === -1) failures.push("a token-showing operation does not refresh lists after showing the token");
      checked++;
    }
    idx += 1;
  }
  if (checked === 0) failures.push("no token-showing operation found");

  globalThis.__uiFailures = failures;
})();
`;
sandbox.__uiSource = jsSource;
vm.runInContext(jsSource + "\n" + harness, sandbox);

// Static HTML/JS contract checks (outside the vm).
const failures = sandbox.__uiFailures || [];
if (!jsSource.includes('<form class="policy-form">')) failures.push("policy form must be a real <form>");
if (jsSource.includes('<button type="button" data-save-policy>')) failures.push("policy save button must be submit-type so the submit handler fires");
if (!/policyForm\.addEventListener\(["']submit["'][^;]*?ev\.preventDefault\(\)/.test(jsSource)) failures.push("policy form must handle submit with preventDefault");

if (failures.length) {
  throw new Error("registration UI regression:\n" + failures.join("\n"));
}
console.log("registration UI regression: ok (token persistence, policy submit contract)");