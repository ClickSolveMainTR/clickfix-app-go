// Snapshot of the last git commit date of every docs page
// (content/docs/last-modified.json), the source of "Last updated" on the
// page. The Docker build context (`frontend/`) has no `.git`, so the image
// build job in CI runs `pnpm docs:dates` on a full-depth checkout right
// before building; the committed file is the fallback every other build
// uses (dev, PR checks). A shallow clone or a checkout without git keeps the
// committed snapshot. Refresh it now and then with `pnpm docs:dates`.
import { execFileSync } from "node:child_process";
import { readdirSync, readFileSync, statSync, writeFileSync } from "node:fs";
import { join, relative } from "node:path";

const root = new URL("../content/docs/", import.meta.url).pathname;
const target = join(root, "last-modified.json");

function git(args) {
  return execFileSync("git", args, {
    cwd: root,
    encoding: "utf8",
    stdio: ["ignore", "pipe", "ignore"],
  }).trim();
}

let shallow = true;
try {
  shallow = git(["rev-parse", "--is-shallow-repository"]) === "true";
} catch {
  console.log("docs:last-modified: no git, snapshot kept");
  process.exit(0);
}
if (shallow) {
  console.log("docs:last-modified: shallow clone, snapshot kept");
  process.exit(0);
}

const current = JSON.parse(readFileSync(target, "utf8"));
const next = {};
function walk(dir) {
  for (const name of readdirSync(dir)) {
    const path = join(dir, name);
    if (statSync(path).isDirectory()) {
      if (name !== "api-reference") walk(path);
      continue;
    }
    if (!name.endsWith(".mdx")) continue;
    const key = relative(root, path).replace(/\\/g, "/");
    const date = git(["log", "-1", "--format=%cI", "--", path]);
    // An uncommitted page keeps its previous date until it is committed.
    next[key] = date || current[key] || new Date().toISOString();
  }
}
walk(root);
const sorted = Object.fromEntries(
  Object.keys(next)
    .sort()
    .map((k) => [k, next[k]]),
);
const text = JSON.stringify(sorted, null, 2) + "\n";
if (text !== readFileSync(target, "utf8")) {
  writeFileSync(target, text);
  console.log("docs:last-modified: snapshot updated");
}
