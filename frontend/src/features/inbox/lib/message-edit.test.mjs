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
    "data:text/javascript;base64," + Buffer.from(code).toString("base64")
  );
}

const { canEditMessage, deliveredToContact, EDIT_WINDOW_MS } =
  await load("./message-edit.ts");

const now = Date.parse("2026-11-18T10:00:00Z");
const reply = {
  direction: "out",
  kind: "text",
  private: false,
  status: "delivered",
  content: "Merhba",
  created_at: "2026-11-18T09:55:00Z",
  sender: { type: "agent", uuid: "me" },
};

test("the author edits a recent public text reply", () => {
  assert.equal(canEditMessage(reply, "me", now), true);
});

test("other authors, notes, templates and contact messages are not editable", () => {
  assert.equal(canEditMessage(reply, "someone-else", now), false);
  assert.equal(canEditMessage(reply, null, now), false);
  assert.equal(
    canEditMessage(
      { ...reply, direction: "internal", private: true, kind: "note" },
      "me",
      now,
    ),
    false,
  );
  assert.equal(
    canEditMessage({ ...reply, kind: "template" }, "me", now),
    false,
  );
  assert.equal(
    canEditMessage(
      { ...reply, direction: "in", sender: { type: "contact", uuid: "me" } },
      "me",
      now,
    ),
    false,
  );
  assert.equal(
    canEditMessage(
      { ...reply, sender: { type: "bot", uuid: "me" } },
      "me",
      now,
    ),
    false,
  );
  assert.equal(
    canEditMessage({ ...reply, optimistic: true }, "me", now),
    false,
  );
  assert.equal(
    canEditMessage({ ...reply, status: "pending" }, "me", now),
    false,
  );
  assert.equal(canEditMessage({ ...reply, content: "" }, "me", now), false);
});

test("the edit window closes after 15 minutes", () => {
  const sent = Date.parse(reply.created_at);
  assert.equal(canEditMessage(reply, "me", sent + EDIT_WINDOW_MS), true);
  assert.equal(canEditMessage(reply, "me", sent + EDIT_WINDOW_MS + 1), false);
});

test("delivered statuses", () => {
  assert.equal(deliveredToContact({ status: "sent" }), true);
  assert.equal(deliveredToContact({ status: "read" }), true);
  assert.equal(deliveredToContact({ status: "queued" }), false);
  assert.equal(deliveredToContact({ status: "failed" }), false);
});
