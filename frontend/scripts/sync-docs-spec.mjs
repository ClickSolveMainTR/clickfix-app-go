import { readFile, writeFile, mkdir } from "node:fs/promises";
import { fileURLToPath } from "node:url";

const frontend = new URL("../", import.meta.url);
const origin = new URL("../backend/docs/public-api.yaml", frontend);
const target = new URL("content/api/public-api.yaml", frontend);
const check = process.argv.includes("--check");
const source = await readFile(origin);
if (check) {
  const copy = await readFile(target);
  if (!source.equals(copy)) {
    throw new Error("Docs API copy is stale. Run pnpm docs:sync.");
  }
} else {
  await mkdir(fileURLToPath(new URL("content/api/", frontend)), {
    recursive: true,
  });
  await writeFile(target, source);
}
