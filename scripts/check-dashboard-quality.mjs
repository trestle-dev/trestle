import {readFile, readdir, stat} from "node:fs/promises";
import {join} from "node:path";

const root = new URL("../internal/web/public/", import.meta.url);
const html = await readFile(new URL("index.html", root), "utf8");
const css = await readFile(new URL("assets/css/style.css", root), "utf8");
const failures = [];
const requireText = (text, pattern, message) => { if (!pattern.test(text)) failures.push(message); };

requireText(html, /<html[^>]+lang="en"/i, "document language is missing");
requireText(html, /name="viewport"/i, "viewport metadata is missing");
requireText(html, /<main[^>]+tabindex="-1"/i, "route focus target is missing");
requireText(html, /<nav[^>]+aria-label=/i, "primary navigation has no accessible name");
requireText(html, /aria-controls="primary-nav"/i, "mobile menu does not name its controlled region");
requireText(html, /role="status"/i, "connection state is not announced");
requireText(css, /:focus-visible/, "visible keyboard focus styling is missing");
requireText(css, /prefers-reduced-motion:reduce/, "reduced-motion handling is missing");
requireText(css, /@media\(max-width:800px\)/, "mobile drawer breakpoint is missing");
requireText(css, /overflow-wrap:anywhere|word-break:break-word/, "long unbroken values are not bounded");
// First-run database-selection structural contract (CP2): exactly one auth
// form that switches between first-administrator creation and sign-in, a
// seven-character password minimum, a present database test/apply button, and
// distinct disabled vs enabled hover styling so an enabled button looks
// interactive.
requireText(html, /<form[^>]*id="auth-form"/, "the auth gate has no single first-run/sign-in form");
if ((html.match(/id="auth-form"/g) || []).length !== 1) failures.push("more than one auth form exists (first-run and sign-in must never both render)");
if ((html.match(/id="auth-submit"/g) || []).length !== 1) failures.push("the auth form must have exactly one submit button");
requireText(html, /id="auth-password"[^>]*minlength="7"/, "the seven-character password minimum is missing from the auth gate");
requireText(html, /id="auth-policy"/, "the first-run application registration policy selector is missing from the auth gate");
requireText(html, /id="database-apply"/, "the database test/apply button is missing from the auth gate");
requireText(css, /button:disabled/, "disabled buttons have no visible disabled styling");
requireText(css, /button:not\(:disabled\):hover/, "enabled buttons have no hover styling");
// Degraded-state operator UX contract (CP12): the status card announces its
// state (aria-live), carries a consequence and a next action, and distinguishes
// ready from database-unavailable copy.
requireText(html, /id="status-card"[^>]*aria-live="polite"/, "the status card does not announce state changes");
requireText(html, /class="next-action"/, "the status card lacks a next-action line");
requireText(css, /\.next-action/, "the next-action line has no styling");
const js = await readFile(new URL("assets/js/script.js", root), "utf8");
requireText(js, /database_unavailable/, "the dashboard does not distinguish database-unavailable readiness");
requireText(js, /TrestleDatabaseSetup\.connectionState/, "the status card does not route through the connection-state machine");
if (/<script(?![^>]+src=)|\sstyle=|\sonclick=/i.test(html)) failures.push("inline executable/style content violates the strict CSP");
if (/<img(?![^>]+alt=)/i.test(html)) failures.push("an image is missing alt text");

async function walk(directory) {
  const files = [];
  for (const entry of await readdir(directory, {withFileTypes:true})) {
    const path = join(directory, entry.name);
    if (entry.isDirectory()) files.push(...await walk(path)); else files.push(path);
  }
  return files;
}
const files = await walk(root.pathname);
let total = 0;
for (const file of files) total += (await stat(file)).size;
if (Buffer.byteLength(html) > 24 * 1024) failures.push("dashboard HTML exceeds 24 KiB");
if (Buffer.byteLength(css) > 96 * 1024) failures.push("dashboard CSS exceeds 96 KiB");
if (total > 320 * 1024) failures.push("embedded dashboard exceeds 320 KiB");

if (failures.length) {
  console.error(failures.map(item => `- ${item}`).join("\n"));
  process.exit(1);
}
console.log(`dashboard quality: ${files.length} files, ${total} bytes, accessibility and responsive contracts present`);
