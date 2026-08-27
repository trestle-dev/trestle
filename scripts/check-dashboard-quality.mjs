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
