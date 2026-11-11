// Copies the generated public API spec (backend/docs/public-api.yaml, English)
// into the docs content and derives the Turkish spec from its `x-i18n`
// extensions (content/api/public-api.tr.yaml). `--check` fails when either
// file is stale (CI `pnpm docs:check`); `pnpm docs:sync` rewrites them.
import { readFile, writeFile, mkdir } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { parse, stringify } from "yaml";

const frontend = new URL("../", import.meta.url);
const origin = new URL("../backend/docs/public-api.yaml", frontend);
const targets = {
  en: new URL("content/api/public-api.yaml", frontend),
  tr: new URL("content/api/public-api.tr.yaml", frontend),
};
const check = process.argv.includes("--check");

/** Apply (or strip) the `x-i18n.<locale>` overlays of every node, in place. */
function localize(node, locale) {
  if (Array.isArray(node)) {
    for (const item of node) localize(item, locale);
    return;
  }
  if (!node || typeof node !== "object") return;
  const i18n = node["x-i18n"];
  if (i18n && typeof i18n === "object") {
    delete node["x-i18n"];
    if (locale !== "en" && i18n[locale]) Object.assign(node, i18n[locale]);
  }
  for (const value of Object.values(node)) localize(value, locale);
}

const source = await readFile(origin, "utf8");
const header = source
  .split("\n")
  .filter((line) => line.startsWith("#"))
  .join("\n");
const outputs = {};
for (const locale of Object.keys(targets)) {
  const doc = parse(source);
  localize(doc, locale);
  const note =
    locale === "en"
      ? "# Docs copy (frontend `pnpm docs:sync`), x-i18n stripped."
      : "# Docs copy in Turkish (frontend `pnpm docs:sync`), x-i18n applied.";
  outputs[locale] =
    `${header}\n${note}\n${stringify(doc, { lineWidth: 0, aliasDuplicateObjects: false })}`;
}

if (check) {
  for (const [locale, target] of Object.entries(targets)) {
    const copy = await readFile(target, "utf8").catch(() => "");
    if (copy !== outputs[locale]) {
      throw new Error(`${fileURLToPath(target)} is stale. Run pnpm docs:sync.`);
    }
  }
} else {
  await mkdir(fileURLToPath(new URL("content/api/", frontend)), {
    recursive: true,
  });
  for (const [locale, target] of Object.entries(targets)) {
    await writeFile(target, outputs[locale]);
  }
}
