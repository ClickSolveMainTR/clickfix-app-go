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

const c = await load("./control.ts");
const noMods = { alt: false, ctrl: false, meta: false, shift: false };

test("points are normalized against the letterboxed frame", () => {
  // 1920x1080 frame in a 1000x1000 box: 562.5 px tall, 218.75 px bars.
  assert.deepEqual(c.normalizePoint(500, 500, 1000, 1000, 1920, 1080), {
    x: 0.5,
    y: 0.5,
  });
  assert.deepEqual(c.normalizePoint(0, 218.75, 1000, 1000, 1920, 1080), {
    x: 0,
    y: 0,
  });
  assert.equal(c.normalizePoint(500, 100, 1000, 1000, 1920, 1080), null);
  assert.equal(c.normalizePoint(10, 10, 0, 0, 1920, 1080), null);
  // Actual size: the element is the frame itself.
  assert.deepEqual(c.normalizePoint(960, 270, 1920, 1080, 1920, 1080), {
    x: 0.5,
    y: 0.25,
  });
});

test("normalized points map back to screen pixels", () => {
  assert.deepEqual(c.toScreen(0, 0, 2560, 1440), { x: 0, y: 0 });
  assert.deepEqual(c.toScreen(1, 1, 2560, 1440), { x: 2559, y: 1439 });
  assert.deepEqual(c.toScreen(2, -1, 100, 100), { x: 99, y: 0 });
});

test("wheel deltas become clamped pixels", () => {
  assert.deepEqual(c.wheelDelta(0, 3, 1), { dx: 0, dy: 48 });
  assert.deepEqual(c.wheelDelta(0, 5000, 0), { dx: 0, dy: 1200 });
  assert.deepEqual(c.wheelDelta(-1, 0, 2), { dx: -800, dy: 0 });
});

test("events round trip with version and sequence", () => {
  const stamp = c.createSequencer(() => 1_700_000_000_000);
  const a = stamp({ type: "pointer.down", x: 0.25, y: 0.75, button: 0 });
  const b = stamp({
    type: "key.down",
    code: "KeyA",
    key: "a",
    mods: noMods,
    repeat: false,
  });
  assert.equal(a.v, c.CONTROL_PROTOCOL_VERSION);
  assert.equal(a.seq, 1);
  assert.equal(b.seq, 2);
  assert.deepEqual(c.decodeEvent(c.encodeEvent(a)), a);
  assert.deepEqual(c.decodeEvent(c.encodeEvent(b)), b);
  assert.deepEqual(
    c.decodeEvent(c.encodeEvent(stamp({ type: "release" }))).type,
    "release",
  );
});

test("receivers drop unknown versions, types and bad values", () => {
  const enc = (o) => new TextEncoder().encode(JSON.stringify(o));
  const base = { v: 1, seq: 1, t: 1 };
  assert.equal(c.decodeEvent(enc({ ...base, v: 2, type: "release" })), null);
  assert.equal(c.decodeEvent(enc({ ...base, type: "clipboard" })), null);
  assert.equal(
    c.decodeEvent(enc({ ...base, type: "pointer.move", x: 1.2, y: 0 })),
    null,
  );
  assert.equal(
    c.decodeEvent(
      enc({ ...base, type: "pointer.down", x: 0, y: 0, button: 4 }),
    ),
    null,
  );
  assert.equal(c.decodeEvent(enc({ ...base, seq: 0, type: "release" })), null);
  assert.equal(c.decodeEvent(new TextEncoder().encode("not json")), null);
  assert.equal(c.decodeEvent(new Uint8Array(c.CONTROL_MAX_BYTES + 1)), null);
  // Extra fields are ignored, not fatal.
  assert.equal(
    c.decodeEvent(enc({ ...base, type: "release", extra: true })).type,
    "release",
  );
});

test("system shortcuts are never sent or applied", () => {
  const ctrlAlt = { ...noMods, ctrl: true, alt: true };
  assert.equal(c.isBlockedCombo("Delete", ctrlAlt), true);
  assert.equal(c.isBlockedCombo("KeyL", { ...noMods, meta: true }), true);
  assert.equal(
    c.isBlockedCombo("Escape", { ...noMods, ctrl: true, shift: true }),
    true,
  );
  assert.equal(c.isBlockedCombo("KeyC", { ...noMods, ctrl: true }), false);
  const enc = new TextEncoder().encode(
    JSON.stringify({
      v: 1,
      seq: 1,
      t: 1,
      type: "key.down",
      code: "Delete",
      key: "Delete",
      mods: ctrlAlt,
    }),
  );
  assert.equal(c.decodeEvent(enc), null);
});
