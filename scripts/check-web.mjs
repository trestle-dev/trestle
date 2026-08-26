import { readFile, readdir, stat } from "node:fs/promises";
import path from "node:path";
import process from "node:process";

const root = path.resolve(process.argv[2] || "public");
const files = [];

async function walk(directory) {
  for (const name of await readdir(directory)) {
    const full = path.join(directory, name);
    const info = await stat(full);
    if (info.isDirectory()) await walk(full);
    else files.push(full);
  }
}

await walk(root);
const html = files.filter((file) => file.endsWith(".html"));
if (html.length === 0) throw new Error(`no HTML files beneath ${root}`);

const missing = [];
for (const file of html) {
  const source = await readFile(file, "utf8");
  for (const match of source.matchAll(/(?:href|src)="([^"#]+)(?:#[^"]*)?"/g)) {
    const value = match[1];
    if (/^(?:https?:|mailto:|data:)/.test(value)) continue;
    if (value.startsWith("/") && value !== "/" && !path.extname(value)) continue;
    let target;
    if (value.startsWith("/")) target = path.join(root, value);
    else target = path.resolve(path.dirname(file), value);
    if (target.endsWith(path.sep)) target = path.join(target, "index.html");
    try {
      await stat(target);
    } catch {
      missing.push(`${path.relative(root, file)} -> ${value}`);
    }
  }
}

if (missing.length) throw new Error(`broken local references:\n${missing.join("\n")}`);
console.log(`checked ${html.length} HTML pages and ${files.length} files`);
