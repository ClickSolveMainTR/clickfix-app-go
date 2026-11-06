import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import { transform } from "esbuild";
import {
  createTable,
  getCoreRowModel,
  getFilteredRowModel,
  getPaginationRowModel,
} from "@tanstack/react-table";

async function load(name) {
  const { code } = await transform(
    await readFile(new URL(name, import.meta.url), "utf8"),
    { loader: "ts", format: "esm" },
  );
  return import(
    `data:text/javascript;base64,${Buffer.from(code).toString("base64")}`
  );
}
const { paginationSummary, pageIndexAfterEmptyPage, pageSizeChoices } =
  await load("./pagination-summary.ts");

/** A server-mode table holding page 2 (20 rows) of a 45-row list. */
function serverTable(options) {
  return createTable({
    data: Array.from({ length: 20 }, (_, i) => ({ id: String(i) })),
    columns: [{ accessorKey: "id" }],
    state: {
      pagination: { pageIndex: 1, pageSize: 20 },
      columnPinning: { left: [], right: [] },
      columnFilters: [],
      globalFilter: "",
      sorting: [],
      columnVisibility: {},
      columnOrder: [],
      rowSelection: {},
      expanded: {},
      grouping: [],
      columnSizing: {},
      rowPinning: {},
    },
    onStateChange() {},
    renderFallbackValue: null,
    manualPagination: true,
    manualFiltering: true,
    manualSorting: true,
    getCoreRowModel: getCoreRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
    ...options,
  });
}

function summaryOf(table) {
  const { pageIndex, pageSize } = table.getState().pagination;
  return paginationSummary({
    pageIndex,
    pageSize,
    manual: Boolean(table.options.manualPagination),
    rowCount: table.options.rowCount,
    pageRows: table.getRowModel().rows.length,
    filteredRows: table.getFilteredRowModel().rows.length,
  });
}

test("server mode: the filtered row model is only the current page", () => {
  // The regression: the counter used this length as the total, which rendered
  // "21–20 / 20" on page 2 of a 45-row list.
  const table = serverTable({ pageCount: 3 });
  assert.equal(table.getFilteredRowModel().rows.length, 20);
});

test("server mode: rowCount drives the counter and the page count", () => {
  const table = serverTable({ rowCount: 45 });
  assert.deepEqual(summaryOf(table), { from: 21, to: 40, total: 45 });
  assert.equal(table.getPageCount(), 3);
  assert.equal(table.getCanNextPage(), true);
});

test("server mode: last page ends at the total", () => {
  assert.deepEqual(
    paginationSummary({
      pageIndex: 2,
      pageSize: 20,
      manual: true,
      rowCount: 45,
      pageRows: 5,
      filteredRows: 5,
    }),
    { from: 41, to: 45, total: 45 },
  );
});

test("server mode without rowCount: range only, no invented total", () => {
  const table = serverTable({ pageCount: 3 });
  assert.deepEqual(summaryOf(table), { from: 21, to: 40, total: null });
});

test("server mode: empty page", () => {
  assert.deepEqual(
    paginationSummary({
      pageIndex: 0,
      pageSize: 20,
      manual: true,
      rowCount: 0,
      pageRows: 0,
      filteredRows: 0,
    }),
    { from: 0, to: 0, total: 0 },
  );
});

test("client mode: total is the filtered row count", () => {
  assert.deepEqual(
    paginationSummary({
      pageIndex: 1,
      pageSize: 10,
      manual: false,
      pageRows: 5,
      filteredRows: 15,
    }),
    { from: 11, to: 15, total: 15 },
  );
  assert.deepEqual(
    paginationSummary({
      pageIndex: 0,
      pageSize: 10,
      manual: false,
      pageRows: 0,
      filteredRows: 0,
    }),
    { from: 0, to: 0, total: 0 },
  );
});

test("server mode: an emptied page falls back to the last page with rows", () => {
  const base = { manual: true, isLoading: false, pageSize: 50 };
  // 51 rows, page 2 holds one; it is deleted -> 50 rows, page 2 is empty.
  assert.equal(
    pageIndexAfterEmptyPage({
      ...base,
      pageIndex: 1,
      pageRows: 0,
      rowCount: 50,
    }),
    0,
  );
  // The list shrank a lot while the user sat on page 5.
  assert.equal(
    pageIndexAfterEmptyPage({
      ...base,
      pageIndex: 4,
      pageRows: 0,
      rowCount: 60,
    }),
    1,
  );
  // No total known: step back one page at a time.
  assert.equal(
    pageIndexAfterEmptyPage({ ...base, pageIndex: 3, pageRows: 0 }),
    2,
  );
  // A stale total that still points at the empty page must not loop.
  assert.equal(
    pageIndexAfterEmptyPage({
      ...base,
      pageIndex: 1,
      pageRows: 0,
      rowCount: 51,
    }),
    0,
  );
});

test("no fallback while loading, on page 1, or with rows", () => {
  const base = { isLoading: false, pageSize: 50, rowCount: 0 };
  assert.equal(
    pageIndexAfterEmptyPage({
      ...base,
      pageIndex: 1,
      pageRows: 0,
      isLoading: true,
    }),
    null,
  );
  assert.equal(
    pageIndexAfterEmptyPage({ ...base, pageIndex: 0, pageRows: 0 }),
    null,
  );
  assert.equal(
    pageIndexAfterEmptyPage({ ...base, pageIndex: 1, pageRows: 3 }),
    null,
  );
});

test("client mode: an edit that empties the page steps back, not to page 1", () => {
  // 41 filtered rows, 20 per page, user on page 3 deletes its only row.
  assert.equal(
    pageIndexAfterEmptyPage({
      isLoading: false,
      pageIndex: 2,
      pageSize: 20,
      pageRows: 0,
      rowCount: 40,
    }),
    1,
  );
});

test("page-size choices always contain the current size, in order", () => {
  assert.deepEqual(
    pageSizeChoices([10, 20, 30, 50, 100], 25),
    [10, 20, 25, 30, 50, 100],
  );
  assert.deepEqual(pageSizeChoices([10, 20, 50], 20), [10, 20, 50]);
  assert.deepEqual(pageSizeChoices([50, 10], 200), [10, 50, 200]);
});

test("resetPagination() drops the page size to 10; setPageIndex(0) keeps it", () => {
  // Why the toolbar's "reset all" uses setPageIndex(0): the table is created
  // without an initial pagination state, so TanStack resets to its default.
  let state = { pageIndex: 2, pageSize: 50 };
  const table = serverTable({
    rowCount: 500,
    onPaginationChange: (updater) => {
      state = typeof updater === "function" ? updater(state) : updater;
    },
  });
  table.resetPagination();
  assert.deepEqual(state, { pageIndex: 0, pageSize: 10 });
  state = { pageIndex: 2, pageSize: 50 };
  table.setPageIndex(0);
  assert.deepEqual(state, { pageIndex: 0, pageSize: 50 });
});
