// Realtime transport view (CP12R3/CP12R4).
//
// Resource ownership is delegated to one module-level TrestleRealtimeController
// singleton so a single route-change listener registered once always cleans up
// the *current* visit's EventSource and staleness interval (CP12R4 blocker 1).
//
// Staleness means "missing transport heartbeat", not merely "no business events
// recently" (CP12R4 blocker 2): the server emits an observable `heartbeat` SSE
// event every 15 seconds; the client's heartbeat listener calls mark() without
// adding an item to the event inspector, so a healthy idle stream stays live.
const rt = TrestleRealtimeController.createController();

function realtimeCleanup() {
  const s = rt.cleanup();
  if (s) { s.onopen = null; s.onerror = null; }
}
window.addEventListener("trestle:viewchange", realtimeCleanup);

function renderRealtime() {
  document.getElementById("overview-content").hidden = true;
  document.getElementById("retry").hidden = true;
  const host = document.getElementById("view-content");
  host.hidden = false;
  host.className = "record-view";
  host.innerHTML = '<div class="record-toolbar"><div><p class="eyebrow">Durable event journal</p><h2>Realtime</h2></div><button type="button" data-pause>Pause</button></div><form class="query-bar realtime-filter"><label>Topic filter<input name="topic" placeholder="record.created"><span>Leave empty to listen to record.created, record.updated and record.deleted.</span></label><button>Reconnect</button></form><p class="connection-state">Connecting…</p><div class="event-inspector" aria-live="polite"></div>';
  const inspector = host.querySelector(".event-inspector");
  // Paused is a per-visit choice (CP12R5): reset it at the start of every
  // Realtime visit so the controller always agrees with the fresh "Pause"
  // button copy, instead of inheriting a previous visit's pause. cleanup()
  // deliberately does not reset it because reconnect() reuses cleanup() and a
  // reconnect within the same visit must preserve the pause state.
  rt.setPaused(false);

  const mark = () => {
    rt.setActivity(Date.now());
    const state = host.querySelector(".connection-state");
    if (state.textContent.indexOf("Stale") === 0) {
      state.textContent = "Connected · live";
    }
  };

  const connect = () => {
    realtimeCleanup();
    const topic = host.querySelector('[name="topic"]').value.trim();
    const source = new EventSource("/api/v1/realtime" + (topic ? "?topic=" + encodeURIComponent(topic) : ""));
    rt.setSource(source);
    rt.setActivity(Date.now());
    source.onopen = () => {
      rt.setActivity(Date.now());
      host.querySelector(".connection-state").textContent = "Connected · replay resumes from the last delivered sequence";
    };
    source.onerror = () => host.querySelector(".connection-state").textContent = "Reconnecting…";
    source.addEventListener("ready", () => {
      mark();
      host.querySelector(".connection-state").textContent = topic
        ? "Connected · listening to " + topic
        : "Connected · listening to all record topics";
    });
    // Transport health: an observable heartbeat keeps the stream live without
    // polluting the event inspector. Business events refresh activity too.
    source.addEventListener("heartbeat", mark);
    ["record.created", "record.updated", "record.deleted"].forEach((name) =>
      source.addEventListener(name, (event) => {
        mark();
        if (rt.paused()) return;
        const item = JSON.parse(event.data);
        const article = document.createElement("article");
        article.innerHTML = `<div><strong>${escapeHTML(item.topic)}</strong><span>#${item.sequence} · ${new Date(item.occurredAt).toLocaleString()}</span></div><pre class="json-block">${highlightJSON(JSON.stringify(item, null, 2))}</pre>`;
        inspector.prepend(article);
        while (inspector.children.length > 200) inspector.lastElementChild.remove();
      })
    );
    rt.setTimer(setInterval(() => {
      if (TrestleDatabaseSetup.staleState(rt.activity(), Date.now(), rt.paused())) {
        host.querySelector(".connection-state").textContent = "Stale · no heartbeat for a while; the connection may have dropped - check it and re-open the stream";
      }
    }, 10000));
  };

  host.querySelector("form").addEventListener("submit", (event) => {
    event.preventDefault();
    inspector.replaceChildren();
    connect();
  });
  host.querySelector("[data-pause]").addEventListener("click", (event) => {
    rt.setPaused(!rt.paused());
    event.currentTarget.textContent = rt.paused() ? "Resume" : "Pause";
  });
  connect();

  // Test/browser-harness hook: lets an external driver force the stale and
  // recovered transitions deterministically without fault-injecting the server.
  window.__trestleRealtime = {
    mark,
    setActivity: (t) => rt.setActivity(t),
    activity: () => rt.activity(),
    paused: () => rt.paused(),
    sourceActive: () => rt.source() !== null
  };
}

document.querySelector('[data-route="realtime"]').addEventListener("click", renderRealtime);
