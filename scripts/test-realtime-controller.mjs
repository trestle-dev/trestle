// CP12R4 realtime lifecycle and heartbeat regression test.
//
// Extracts the real realtime and state-machine blocks from the canonical
// embedded public bundle into a VM sandbox with a counting EventSource,
// then proves:
//
//   Lifecycle (blocker 1): enter / reconnect repeatedly / leave / re-enter /
//     leave leaves zero live EventSources and zero intervals after each visit,
//     and exactly one source and one interval exist during a visit. The
//     route-change cleanup listener is registered exactly once.
//
//   Heartbeat (blocker 2): an observable heartbeat keeps an idle stream live
//     without adding an inspector item; business events also refresh activity;
//     missing heartbeats beyond the threshold produce stale; a heartbeat after
//     stale restores connected status; pause never reports a healthy
//     connection as failed; reconnect/error states stay distinct from stale.
import {readFile} from "node:fs/promises";
import vm from "node:vm";

const bundle = await readFile(
  new URL("../internal/web/public/assets/js/script.js", import.meta.url),
  "utf8"
);
const block = (startText, endText) => {
  const start = bundle.indexOf(startText);
  const end = bundle.indexOf(endText, start);
  if (start < 0 || end < 0) throw new Error(`public bundle block missing: ${startText}`);
  return bundle.slice(start, end);
};
const sources = {
  state: block("globalThis.TrestleDatabaseSetup = (() => {", "function applyConnection"),
  controller: block("globalThis.TrestleRealtimeController = (() => {", "// Realtime transport view"),
  realtime: block("// Realtime transport view", "async function renderAudit"),
};

let failures = 0;
const fail = (label, detail) => {
  failures += 1;
  console.error(`- ${label}: ${detail}`);
};
const eq = (label, got, want) => {
  if (got !== want) fail(label, `got ${JSON.stringify(got)}, want ${JSON.stringify(want)}`);
};

// --- Sandbox with counting transport primitives ---------------------------
let intervalId = 0;
const intervalRegistry = new Map(); // id -> fn
const fakeTimers = {set: [], clear: []};
let fakeNow = 1_700_000_000_000;

class FakeEventSource {
  static live = 0;
  static instances = [];
  constructor(url) {
    this.url = url;
    this.listeners = {};
    this.closed = false;
    this.onopen = null;
    this.onerror = null;
    FakeEventSource.live += 1;
    FakeEventSource.instances.push(this);
  }
  addEventListener(type, fn) { (this.listeners[type] ||= []).push(fn); }
  fire(type, ev) { for (const fn of this.listeners[type] || []) fn(ev); }
  close() {
    if (!this.closed) { this.closed = true; FakeEventSource.live -= 1; }
  }
}

function makeEl() {
  const node = {
    hidden: false,
    className: "",
    textContent: "",
    value: "",
    innerHTML: "",
    children: [],
    lastElementChild: null,
    _listeners: {},
    _map: {},
    addEventListener(type, fn) { (node._listeners[type] ||= []).push(fn); },
    fire(type, ev) { for (const fn of node._listeners[type] || []) fn(ev); },
    querySelector(sel) {
      if (!node._map[sel]) node._map[sel] = makeEl();
      return node._map[sel];
    },
    prepend(el) { node.children.unshift(el); node.lastElementChild = node.children[node.children.length - 1]; },
    replaceChildren() { node.children.length = 0; node.lastElementChild = null; },
    remove() {},
    click() {},
    requestSubmit() {}
  };
  return node;
}

const elCache = {};
const routeLink = makeEl();
const document = {
  getElementById(id) {
    if (!elCache["#" + id]) elCache["#" + id] = makeEl();
    return elCache["#" + id];
  },
  createElement() { return makeEl(); },
  querySelector(sel) {
    if (sel === '[data-route="realtime"]') return routeLink;
    if (!elCache[sel]) elCache[sel] = makeEl();
    return elCache[sel];
  }
};

const viewChangeListeners = [];
const windowStub = {
  addEventListener(type, fn) {
    if (type === "trestle:viewchange") viewChangeListeners.push(fn);
  }
};

const RealDate = Date;
const SandboxDate = function (...args) { return new RealDate(...args); };
SandboxDate.now = () => fakeNow;
SandboxDate.prototype = RealDate.prototype;
SandboxDate.parse = RealDate.parse;

const sandbox = {
  window: windowStub,
  document,
  EventSource: FakeEventSource,
  setInterval(fn) { const id = ++intervalId; intervalRegistry.set(id, fn); fakeTimers.set.push(id); return id; },
  clearInterval(id) { intervalRegistry.delete(id); fakeTimers.clear.push(id); },
  Date: SandboxDate,
  escapeHTML: (s) => String(s),
  highlightJSON: (s) => String(s),
  console,
};
sandbox.globalThis = sandbox;
vm.createContext(sandbox);
vm.runInContext(sources.state, sandbox);
vm.runInContext(sources.controller, sandbox);
vm.runInContext(sources.realtime, sandbox);

const host = document.getElementById("view-content");
const stateEl = host.querySelector(".connection-state");
const inspector = host.querySelector(".event-inspector");
const pauseEl = host.querySelector("[data-pause]");
const formEl = host.querySelector("form");
const topicInput = host.querySelector('[name="topic"]');
const renderRealtime = sandbox.renderRealtime;
const machine = sandbox.TrestleDatabaseSetup;
const liveSources = () => FakeEventSource.live;
const liveIntervals = () => intervalRegistry.size;

// --- Lifecycle regression (blocker 1) -------------------------------------
renderRealtime(); // enter visit 1
const hook = sandbox.window.__trestleRealtime;
if (!hook) fail("browser hook", "__trestleRealtime was not exposed");
eq("visit1: one source", liveSources(), 1);
eq("visit1: one interval", liveIntervals(), 1);
eq("cleanup listener registered once", viewChangeListeners.length, 1);
const source1 = FakeEventSource.instances.at(-1);

// Reconnect repeatedly: each submit reconnects, replacing the source and timer.
for (let i = 0; i < 3; i++) {
  formEl.fire("submit", {preventDefault() {}});
}
eq("after reconnects: source1 closed", source1.closed, true);
eq("after reconnects: still one live source", liveSources(), 1);
eq("after reconnects: still one interval", liveIntervals(), 1);

// Leave: the single route-change listener must clear the *current* pair.
viewChangeListeners[0]();
eq("after leave visit1: no live sources", liveSources(), 0);
eq("after leave visit1: no intervals", liveIntervals(), 0);

// Re-enter and leave again: the same once-registered listener still cleans up
// the second visit's fresh resources (the original CP12R4 leak).
renderRealtime();
eq("visit2: one source", liveSources(), 1);
eq("visit2: one interval", liveIntervals(), 1);
eq("still exactly one cleanup listener", viewChangeListeners.length, 1);
const source2 = FakeEventSource.instances.at(-1);
viewChangeListeners[0]();
eq("after leave visit2: no live sources", liveSources(), 0);
eq("after leave visit2: no intervals", liveIntervals(), 0);
eq("after leave visit2: source2 closed", source2.closed, true);

// --- Heartbeat regression (blocker 2) -------------------------------------
renderRealtime(); // active visit 3 for the transport-health tests
const source = FakeEventSource.instances.at(-1);

// The server emits ready immediately, even when the journal is empty, so an
// unfiltered visit becomes visibly connected without waiting for a heartbeat.
source.fire("ready", {});
eq("empty filter connects to all record topics", stateEl.textContent, "Connected · listening to all record topics");
eq("empty filter sends no topic query", source.url, "/api/v1/realtime");

// Heartbeat keeps an otherwise idle stream healthy, without polluting the
// inspector.
const inspectorBefore = inspector.children.length;
source.fire("heartbeat", {});
eq("heartbeat adds no inspector item", inspector.children.length, inspectorBefore);
eq("heartbeat refreshes activity", hook.activity() >= fakeNow - 50, true);
fakeNow += 1000;
eq("idle-but-heartbeated stream not stale", machine.staleState(hook.activity(), fakeNow, false), false);

// Business events also refresh activity (and do add an inspector item).
fakeNow += 1000;
source.fire("record.created", {data: JSON.stringify({topic: "record.created", sequence: 1, occurredAt: new Date(0).toISOString()})});
eq("business event adds inspector item", inspector.children.length, inspectorBefore + 1);
eq("business event refreshes activity", machine.staleState(hook.activity(), fakeNow, false), false);

// Missing heartbeats beyond the threshold produce stale.
const tick = () => intervalRegistry.values().next().value();
hook.setActivity(fakeNow - 31000);
fakeNow += 1000;
tick();
eq("missing heartbeat beyond threshold -> stale", stateEl.textContent.slice(0, 5), "Stale");

// A heartbeat after stale restores connected status.
source.fire("heartbeat", {});
eq("heartbeat after stale restores connected", stateEl.textContent, "Connected · live");

// Pause semantics never report a healthy connection as failed.
hook.setActivity(fakeNow - 40000);
eq("paused suppresses stale (machine)", machine.staleState(hook.activity(), fakeNow, true), false);
pauseEl.fire("click", {currentTarget: pauseEl});
eq("pause toggles controller paused", hook.paused(), true);
eq("pause button label", pauseEl.textContent, "Resume");
stateEl.textContent = "Connected · replay resumes from the last delivered sequence";
fakeNow += 1000;
tick();
eq("paused visit not reported stale", stateEl.textContent.slice(0, 5), "Conne");
pauseEl.fire("click", {currentTarget: pauseEl});
eq("resume toggles controller paused", hook.paused(), false);

// Reconnect/error states remain distinct from stale: onerror sets the
// Reconnecting label, never the stale label.
source.onerror();
eq("onerror sets Reconnecting, not stale", stateEl.textContent, "Reconnecting…");
source.onopen();
eq("onopen restores Connected", stateEl.textContent.slice(0, 9), "Connected");

// --- Pause is a per-visit choice (CP12R5) ---------------------------------
// Pause the current visit, then leave and re-enter: the controller must be
// unpaused and the fresh button copy must say "Pause", so a business event
// appears immediately instead of being suppressed by a stale pause flag.
pauseEl.fire("click", {currentTarget: pauseEl});
eq("paused during the visit", hook.paused(), true);
eq("pause button label while paused", pauseEl.textContent, "Resume");
viewChangeListeners[0](); // leave
eq("after leave: no live sources", liveSources(), 0);
eq("after leave: no intervals", liveIntervals(), 0);

renderRealtime(); // re-enter
eq("re-entered visit starts unpaused", hook.paused(), false);
eq("fresh button copy says Pause",
  host.innerHTML.indexOf('data-pause>Pause</button>') !== -1, true);
const reenteredSource = FakeEventSource.instances.at(-1);
const inspectorBeforeEvent = inspector.children.length;
reenteredSource.fire("record.created", {data: JSON.stringify({topic: "record.created", sequence: 9, occurredAt: new Date(0).toISOString()})});
eq("business event appears immediately on re-entry", inspector.children.length, inspectorBeforeEvent + 1);
eq("re-entered visit has exactly one live source", liveSources(), 1);
eq("re-entered visit has exactly one interval", liveIntervals(), 1);

// Teardown the active visit.
viewChangeListeners[0]();
eq("final teardown: no live sources", liveSources(), 0);
eq("final teardown: no intervals", liveIntervals(), 0);

if (failures) {
  console.error(`realtime lifecycle/heartbeat regression: ${failures} failure(s)`);
  process.exit(1);
}
console.log("realtime lifecycle/heartbeat regression passed");
