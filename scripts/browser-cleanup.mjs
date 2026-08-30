// Owned-process-group cleanup helper (CP12R5).
//
// The caller spawns its managed child with `detached: true`, so the child PID
// is the ID of a new process group that contains only that child's tree.
// signalGroup() sends a signal to that owned group only and resolves as soon as
// the child has exited, or after a bounded wait - it can never hang, because it
// checks exitCode/signalCode before installing the exit listener and installs
// that listener before delivering the signal, so an early or mid-flight exit
// is always observed. The caller should escalate from SIGTERM to SIGKILL only
// after a signalGroup timeout, then rely on the same bound.

export function alreadyExited(proc) {
  return proc.exitCode !== null || proc.signalCode !== null;
}

export function signalGroup(proc, signal, timeoutMs = 5000) {
  if (alreadyExited(proc)) return Promise.resolve(true);
  return new Promise((resolve) => {
    let settled = false;
    const finish = (v) => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      resolve(v);
    };
    const timer = setTimeout(() => finish(false), timeoutMs);
    proc.once("exit", () => finish(true));
    try {
      process.kill(-proc.pid, signal);
    } catch (err) {
      // ESRCH: the owned group is already gone (child exited between the
      // exitCode check and the kill); treat it as exited.
      if (err && err.code === "ESRCH") finish(true);
      else finish(false);
    }
  });
}