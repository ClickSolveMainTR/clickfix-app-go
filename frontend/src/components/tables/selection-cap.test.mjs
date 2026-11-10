import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import { transform } from "esbuild";

const { code } = await transform(
  await readFile(new URL("./selection-cap.ts", import.meta.url), "utf8"),
  { loader: "ts", format: "esm" },
);
const { capSelection, selectedCount } = await import(
  `data:text/javascript;base64,${Buffer.from(code).toString("base64")}`
);

test("selectedCount ignores rows set to false", () => {
  assert.equal(selectedCount({ a: true, b: false, c: true }), 2);
});

test("capSelection leaves a selection under the ceiling alone", () => {
  const next = { a: true, b: true };
  assert.equal(capSelection({ a: true }, next, 3), next);
});

test("capSelection keeps the rows already selected first", () => {
  const out = capSelection(
    { c: true },
    { a: true, b: true, c: true, d: true },
    2,
  );
  assert.deepEqual(out, { c: true, a: true });
});

test("capSelection fills the ceiling from new rows in order", () => {
  assert.deepEqual(capSelection({}, { a: true, b: true, c: true }, 2), {
    a: true,
    b: true,
  });
});

test("capSelection drops rows the update deselected", () => {
  assert.deepEqual(
    capSelection(
      { a: true, b: true },
      { a: false, b: true, c: true, d: true },
      2,
    ),
    { b: true, c: true },
  );
});
