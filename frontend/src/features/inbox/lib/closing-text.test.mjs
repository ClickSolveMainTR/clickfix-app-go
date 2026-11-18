import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import { transform } from "esbuild";

const { code } = await transform(
  await readFile(new URL("./closing-text.ts", import.meta.url), "utf8"),
  { loader: "ts", format: "esm" },
);
const { closingState, NO_CLOSING } = await import(
  "data:text/javascript;base64," + Buffer.from(code).toString("base64")
);

const data = {
  items: [
    { uuid: "a", text: "Saygılarımla, Ayşe" },
    { uuid: "b", text: "Acme ekibi" },
  ],
  selected: "b",
};

test("closing text starts with the server's pick and sends it", () => {
  const s = closingState(data, null, true);
  assert.equal(s.value, "b");
  assert.equal(s.preview, "Acme ekibi");
  assert.equal(s.send, "b");
});

test("the agent's pick wins; none sends none without a preview", () => {
  assert.equal(closingState(data, "a", true).send, "a");
  const none = closingState(data, NO_CLOSING, true);
  assert.equal(none.send, NO_CLOSING);
  assert.equal(none.preview, undefined);
  assert.equal(
    closingState({ ...data, selected: null }, null, true).send,
    NO_CLOSING,
  );
});

test("notes, empty lists and unknown picks", () => {
  assert.equal(closingState(data, null, false).send, undefined);
  assert.equal(
    closingState({ items: [], selected: null }, null, true).show,
    false,
  );
  assert.equal(closingState(undefined, null, true).send, undefined);
  assert.equal(closingState(data, "gone", true).value, NO_CLOSING);
});
