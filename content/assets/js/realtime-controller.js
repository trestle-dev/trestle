// Realtime transport resource controller (DOM-free, clock/registry-injectable).
//
// A Realtime visit can own at most three resources: the EventSource, the
// staleness interval, and a last-activity timestamp. This controller is a
// module-level singleton in realtime.js, so cleanup() always clears the
// *current* pair of resources. A route-change listener registered once against
// the stable cleanup() can therefore never leak a later visit's resources
// (CP12R4: after any enter/leave cycle there are zero sources and zero
// intervals, and during a visit there is exactly one of each).
//
// env is injectable (globalThis by default) so a regression can count real
// setInterval/clearInterval traffic and EventSource instances instead of
// trusting the controller's own counters.
globalThis.TrestleRealtimeController = (() => {
  function createController(env) {
    env = env || globalThis;
    let source = null;
    let timer = null;
    let lastActivity = 0;
    let paused = false;
    return {
      setSource(s) { source = s; },
      source() { return source; },
      setTimer(t) { timer = t; },
      timer() { return timer; },
      setActivity(t) { lastActivity = t; },
      activity() { return lastActivity; },
      setPaused(v) { paused = v; },
      paused() { return paused; },
      // cleanup returns the closed source so a caller can null its handlers
      // before/after close if it wants; the resource itself is always released.
      cleanup() {
        const s = source;
        source = null;
        if (s && typeof s.close === "function") { s.close(); }
        if (timer) { env.clearInterval(timer); timer = null; }
        return s;
      },
      active() { return source !== null || timer !== null; }
    };
  }
  return { createController };
})();