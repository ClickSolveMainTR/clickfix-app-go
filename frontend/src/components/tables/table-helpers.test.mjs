import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import { transform } from "esbuild";

async function load(name) {
  const { code } = await transform(
    await readFile(new URL(name, import.meta.url), "utf8"),
    { loader: "ts", format: "esm" },
  );
  return import(
    `data:text/javascript;base64,${Buffer.from(code).toString("base64")}`
  );
}
const { normalizeSearchText, searchTextIncludes } =
  await load("./search-text.ts");
const { toPersistedState, fromPersistedState } =
  await load("./persist-state.ts");
const { isRowKeyActivation } = await load("./row-click.ts");

test("search: Turkish dotted and dotless i match in both directions", () => {
  assert.ok(searchTextIncludes("İade talebi", "iade"));
  assert.ok(searchTextIncludes("iade talebi", "İADE"));
  assert.ok(searchTextIncludes("Isı pompası", "ısı"));
  assert.ok(searchTextIncludes("ısı pompası", "ISI"));
  // A user on a keyboard without the Turkish layout.
  assert.ok(searchTextIncludes("Isı pompası", "isi"));
  assert.ok(searchTextIncludes("IADE", "iade"));
});

test("search: case and diacritics are ignored, other text still matters", () => {
  assert.ok(searchTextIncludes("Şikâyet", "sikayet"));
  assert.ok(searchTextIncludes("Ödeme güncelleme", "ODEME GUNCELLEME"));
  assert.ok(searchTextIncludes("Çağrı", "cagri"));
  assert.ok(searchTextIncludes("INFO", "info"));
  assert.ok(!searchTextIncludes("İade", "ödeme"));
  assert.equal(normalizeSearchText("İstanbul IĞDIR"), "istanbul igdir");
});

test("search: empty query matches, numbers and null are handled", () => {
  assert.ok(searchTextIncludes("anything", ""));
  assert.ok(searchTextIncludes("anything", "   "));
  assert.ok(searchTextIncludes(1250, "25"));
  assert.ok(!searchTextIncludes(null, "x"));
});

test("persist: search text and page index are not stored, preferences are", () => {
  const stored = toPersistedState({
    sorting: [{ id: "name", desc: false }],
    columnVisibility: { email: false },
    globalFilter: "iade",
    pagination: { pageIndex: 4, pageSize: 50 },
    density: "compact",
    viewMode: "grid",
  });
  assert.equal("globalFilter" in stored, false);
  assert.deepEqual(stored.pagination, { pageIndex: 0, pageSize: 50 });
  assert.deepEqual(stored.sorting, [{ id: "name", desc: false }]);
  assert.deepEqual(stored.columnVisibility, { email: false });
  assert.equal(stored.density, "compact");
  assert.equal(stored.viewMode, "grid");
});

test("persist: values stored by the older version are cleaned on read", () => {
  const restored = fromPersistedState({
    globalFilter: "eski arama",
    pagination: { pageIndex: 7, pageSize: 20 },
    viewMode: "table",
  });
  assert.equal("globalFilter" in restored, false);
  assert.deepEqual(restored.pagination, { pageIndex: 0, pageSize: 20 });
  assert.equal(restored.viewMode, "table");
  assert.equal(fromPersistedState(null), null);
  assert.equal(fromPersistedState("x"), null);
});

test("row keyboard: Enter and Space open the row only from the row itself", () => {
  const row = {};
  const inner = {};
  const key = (k, target = row, extra = {}) => ({
    key: k,
    defaultPrevented: false,
    target,
    currentTarget: row,
    ...extra,
  });
  assert.ok(isRowKeyActivation(key("Enter")));
  assert.ok(isRowKeyActivation(key(" ")));
  assert.ok(!isRowKeyActivation(key("a")));
  assert.ok(!isRowKeyActivation(key("Tab")));
  // Enter on a button / link inside the row belongs to that control.
  assert.ok(!isRowKeyActivation(key("Enter", inner)));
  assert.ok(!isRowKeyActivation(key(" ", inner)));
  assert.ok(!isRowKeyActivation(key("Enter", row, { defaultPrevented: true })));
});
