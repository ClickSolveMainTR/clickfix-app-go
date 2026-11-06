import assert from "node:assert/strict";
import { mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import { fileURLToPath, pathToFileURL } from "node:url";
import { build } from "esbuild";

const src = fileURLToPath(new URL("../../", import.meta.url));

/**
 * Bundles the real components (EntityTable -> DataTable -> pagination) with
 * one stub: the locale provider, whose `t` echoes the key and its variables so
 * the counter can be read from the markup.
 */
async function loadComponents() {
  const { outputFiles } = await build({
    stdin: {
      contents: `
        export { createElement } from "react";
        export { renderToStaticMarkup } from "react-dom/server";
        export { EntityTable } from "@/components/entity/entity-table";
        export { DataTable } from "@/components/tables/data-table";
      `,
      resolveDir: src,
      loader: "ts",
    },
    bundle: true,
    write: false,
    format: "esm",
    platform: "node",
    jsx: "automatic",
    alias: { "@": src.replace(/\/$/, "") },
    banner: {
      js: 'import { createRequire as __cr } from "node:module"; const require = __cr(import.meta.url);',
    },
    define: { "process.env.NODE_ENV": '"production"' },
    logLevel: "silent",
    plugins: [
      {
        name: "stub-locale",
        setup(b) {
          b.onResolve({ filter: /^@\/providers\/locale-provider$/ }, () => ({
            path: "locale-stub",
            namespace: "stub",
          }));
          b.onLoad({ filter: /.*/, namespace: "stub" }, () => ({
            contents: `export function useLocale() {
              return {
                locale: "en",
                t: (key, vars) => (vars ? key + JSON.stringify(vars) : key),
              };
            }`,
            loader: "js",
          }));
        },
      },
    ],
  });
  const dir = await mkdtemp(join(tmpdir(), "tables-test-"));
  const file = join(dir, "bundle.mjs");
  await writeFile(file, outputFiles[0].text);
  try {
    return await import(pathToFileURL(file).href);
  } finally {
    await rm(dir, { recursive: true, force: true });
  }
}

const { createElement, renderToStaticMarkup, EntityTable, DataTable } =
  await loadComponents();

const columns = [{ accessorKey: "id", header: "id" }];
const page = Array.from({ length: 20 }, (_, i) => ({ id: String(i + 21) }));
const text = (html) => html.replaceAll("&quot;", '"');

test("EntityTable total reaches the counter: page 2 of 45 rows", () => {
  const html = text(
    renderToStaticMarkup(
      createElement(EntityTable, {
        columns,
        data: page,
        total: 45,
        state: {
          pagination: { pageIndex: 1, pageSize: 20 },
          onPaginationChange() {},
        },
      }),
    ),
  );
  assert.ok(
    html.includes('table.showing{"from":21,"to":40,"total":45}'),
    "counter shows 21–40 of 45",
  );
  // Page indicator derived from the total, not passed by the screen.
  assert.match(html, />2(<!-- -->)?\/(<!-- -->)?3</, "page 2 of 3");
});

test("server mode without a total shows the range only", () => {
  const html = text(
    renderToStaticMarkup(
      createElement(EntityTable, {
        columns,
        data: page,
        pageCount: 3,
        state: {
          pagination: { pageIndex: 1, pageSize: 20 },
          onPaginationChange() {},
        },
      }),
    ),
  );
  assert.ok(html.includes('table.showing_range{"from":21,"to":40}'));
  assert.ok(!html.includes("table.showing{"));
});

test("client mode counts the rows it holds", () => {
  const html = text(
    renderToStaticMarkup(
      createElement(DataTable, {
        columns,
        data: Array.from({ length: 25 }, (_, i) => ({ id: String(i) })),
        initialState: { pagination: { pageIndex: 1, pageSize: 10 } },
      }),
    ),
  );
  assert.ok(html.includes('table.showing{"from":11,"to":20,"total":25}'));
});
