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
        export { createElement, useState } from "react";
        export { useDataTable } from "@/components/tables/use-data-table";
        export { tableEmptyPageFallback } from "@/components/tables/empty-page";
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

const {
  createElement,
  useState,
  useDataTable,
  tableEmptyPageFallback,
  renderToStaticMarkup,
  EntityTable,
  DataTable,
} = await loadComponents();

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

const rows = [
  { id: "1", name: "İade" },
  { id: "2", name: "Ödeme" },
];
const nameColumns = [{ accessorKey: "name", header: "name" }];

test("rows with onRowClick are focusable and announce how to open", () => {
  const html = text(
    renderToStaticMarkup(
      createElement(DataTable, {
        columns: nameColumns,
        data: rows,
        onRowClick() {},
      }),
    ),
  );
  const bodyRows = html.match(/<tr[^>]*tabindex="0"[^>]*>/g) ?? [];
  assert.equal(bodyRows.length, 2, "both data rows are tab stops");
  assert.ok(bodyRows.every((row) => row.includes("aria-describedby=")));
  assert.ok(html.includes("table.row_open_hint"));
});

test("rows without onRowClick are not tab stops", () => {
  const html = text(
    renderToStaticMarkup(
      createElement(DataTable, { columns: nameColumns, data: rows }),
    ),
  );
  assert.equal((html.match(/<tr[^>]*tabindex=/g) ?? []).length, 0);
  assert.ok(!html.includes("table.row_open_hint"));
});

test("cards with onRowClick are focusable too", () => {
  const html = text(
    renderToStaticMarkup(
      createElement(DataTable, {
        columns: nameColumns,
        data: rows,
        onRowClick() {},
        initialState: { viewMode: "grid" },
      }),
    ),
  );
  assert.equal((html.match(/<article[^>]*tabindex="0"/g) ?? []).length, 2);
});

test("loading keeps the toolbar mounted and hides the pagination", () => {
  const html = text(
    renderToStaticMarkup(
      createElement(EntityTable, {
        columns: nameColumns,
        data: [],
        isLoading: true,
        searchPlaceholder: "ara",
      }),
    ),
  );
  assert.ok(html.includes('placeholder="ara"'), "search box is rendered");
  assert.ok(html.includes('aria-busy="true"'), "rows are a skeleton");
  assert.ok(!html.includes("table.showing"));
});

test("an error keeps the toolbar above the error state", () => {
  const html = text(
    renderToStaticMarkup(
      createElement(EntityTable, {
        columns: nameColumns,
        data: [],
        isError: true,
        errorTitle: "yüklenemedi",
        searchPlaceholder: "ara",
      }),
    ),
  );
  assert.ok(html.includes('placeholder="ara"'));
  assert.ok(html.includes('role="alert"'));
  assert.ok(html.indexOf('placeholder="ara"') < html.indexOf('role="alert"'));
});

test("client search is Turkish-aware", () => {
  const html = text(
    renderToStaticMarkup(
      createElement(DataTable, {
        columns: nameColumns,
        data: rows,
        initialState: { globalFilter: "iade" },
      }),
    ),
  );
  assert.ok(html.includes("İade"));
  assert.ok(!html.includes("Ödeme"));
});

/**
 * Drives the real `useDataTable` hook through a list of steps, one per
 * render (server rendering re-renders a component that sets its own state
 * while rendering), and records the pagination after each step.
 */
function runTableSteps({ rowsCount = 25, pageSize = 10, steps }) {
  const seen = [];
  const make = (n) =>
    Array.from({ length: n }, (_, i) => ({
      id: String(i),
      name: `Kayıt ${String(i).padStart(2, "0")}`,
    }));
  function Harness() {
    const [data, setData] = useState(() => make(rowsCount));
    const [step, setStep] = useState(0);
    const { table } = useDataTable({
      columns: nameColumns,
      data,
      getRowId: (row) => row.id,
      initialState: { pagination: { pageIndex: 0, pageSize } },
    });
    const state = table.getState();
    seen[step] = {
      pageIndex: state.pagination.pageIndex,
      pageSize: state.pagination.pageSize,
      autoReset: table.options.autoResetPageIndex,
    };
    if (step < steps.length) {
      steps[step]({ table, setData, make });
      setStep(step + 1);
    }
    return null;
  }
  renderToStaticMarkup(createElement(Harness));
  return seen;
}

test("client mode: search, column filter and sorting go back to page 1", () => {
  for (const change of [
    ({ table }) => table.setGlobalFilter("kayıt"),
    ({ table }) => table.setColumnFilters([{ id: "name", value: "1" }]),
    ({ table }) => table.setSorting([{ id: "name", desc: true }]),
  ]) {
    const seen = runTableSteps({
      steps: [({ table }) => table.setPageIndex(2), change],
    });
    assert.equal(seen[1].pageIndex, 2, "moved to page 3 first");
    assert.equal(seen[2].pageIndex, 0, "the change resets the page");
    assert.equal(seen[2].pageSize, 10, "and keeps the page size");
  }
});

test("client mode: a data refresh keeps the page", () => {
  const seen = runTableSteps({
    steps: [
      ({ table }) => table.setPageIndex(2),
      // An edit or a toggle refetches the list: same rows, new array.
      ({ setData, make }) => setData(make(25)),
    ],
  });
  assert.equal(seen[2].pageIndex, 2);
  // TanStack resets the page in a microtask after data changes unless this
  // is off; the microtask cannot be observed here, the option can.
  assert.equal(seen[2].autoReset, false);
});

test("client mode: an emptied last page is pulled back to the last page with rows", () => {
  // `tableEmptyPageFallback` is what DataTable applies after every render.
  const fallback = {};
  const at =
    (key) =>
    ({ table }) => {
      fallback[key] = tableEmptyPageFallback(table, {
        busy: false,
        serverRows: 0,
      });
    };
  runTableSteps({
    steps: [
      ({ table }) => table.setPageIndex(2),
      at("full"),
      // Page 3 of 25 rows (10 per page) holds 5; the list shrinks to 20.
      ({ setData, make }) => setData(make(20)),
      at("shrunk"),
      // A search that leaves 2 rows while the user is still on page 3
      // (cannot happen through the toolbar, which resets the page).
      ({ setData, make }) => setData(make(2)),
      at("almostEmpty"),
    ],
  });
  assert.deepEqual(fallback, { full: null, shrunk: 1, almostEmpty: 0 });
});

test("empty-page fallback waits while the list is loading", () => {
  const fallback = {};
  runTableSteps({
    steps: [
      ({ table }) => table.setPageIndex(2),
      ({ setData, make }) => setData(make(0)),
      ({ table }) => {
        fallback.busy = tableEmptyPageFallback(table, {
          busy: true,
          serverRows: 0,
        });
      },
    ],
  });
  assert.equal(fallback.busy, null);
});

test("columns can be hidden only when the column menu can bring them back", () => {
  const canHide = (features) => {
    let result;
    function Probe() {
      const { table } = useDataTable({
        columns: nameColumns,
        data: rows,
        features,
      });
      result = table.getAllLeafColumns().map((column) => column.getCanHide());
      return null;
    }
    renderToStaticMarkup(createElement(Probe));
    return result;
  };
  assert.deepEqual(canHide(undefined), [true]);
  assert.deepEqual(canHide({ columnVisibility: false }), [false]);
  // No toolbar, no column menu: a hidden column could never come back.
  assert.deepEqual(canHide({ toolbar: false }), [false]);
});

test("the header offers no hide action on a table without a toolbar", () => {
  const html = (features) =>
    text(
      renderToStaticMarkup(
        createElement(DataTable, {
          columns: [
            { accessorKey: "name", header: "name", enableSorting: false },
          ],
          data: rows,
          features: { columnPinning: false, ...features },
        }),
      ),
    );
  // With nothing left to do (no sort, no pin, no hide) the header is plain
  // text; otherwise it is a menu button.
  assert.ok(!html({ toolbar: false }).includes("aria-haspopup"));
});
