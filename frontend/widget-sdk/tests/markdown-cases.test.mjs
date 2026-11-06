// The widget parser and the Go parser of the channel translator
// (backend/internal/platform/channels/format, CF-177) read the same cases,
// so a message is bold in the widget exactly when it is bold on a channel.
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import { load } from "./load.mjs";

const markdown = await load("../src/shared/markdown.ts");
const cases = JSON.parse(
  await readFile(
    new URL(
      "../../../backend/internal/platform/channels/format/testdata/markdown_cases.json",
      import.meta.url,
    ),
    "utf8",
  ),
);

test("markdown parses like the channel translator (shared fixture)", () => {
  assert.ok(cases.length > 30);
  for (const c of cases)
    assert.deepEqual(markdown.parse(c.src), c.blocks, c.name);
});
