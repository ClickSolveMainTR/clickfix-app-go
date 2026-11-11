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

const {
  idle,
  request,
  cancel,
  confirm,
  dialogCopy,
  reasonTooLong,
  visibilityProblem,
} = await load("./takedown.ts");

const topic = { action: "unpublish", kind: "topic", uuid: "t-1" };

test("the button only stages the request; nothing is sent before confirm", () => {
  const staged = request(idle, topic);
  assert.deepEqual(staged.pending, topic);
  // Pressing the button is not the action.
  assert.equal(confirm(idle).send, null);
});

test("cancel drops the request and sends nothing", () => {
  const state = cancel(request(idle, topic));
  assert.equal(state.pending, null);
  assert.equal(confirm(state).send, null);
});

test("confirm sends the staged request once, with the trimmed reason", () => {
  const { send, state } = confirm(request(idle, topic), "  spam  ");
  assert.deepEqual(send, { ...topic, reason: "spam" });
  assert.equal(state.pending, null, "the dialog closes");
  assert.equal(
    confirm(state, "again").send,
    null,
    "a second confirm sends nothing",
  );
});

test("republish carries no reason", () => {
  const { send } = confirm(
    request(idle, { action: "republish", kind: "reply", uuid: "r-1" }),
    "ignored",
  );
  assert.deepEqual(send, {
    action: "republish",
    kind: "reply",
    uuid: "r-1",
    reason: "",
  });
});

test("the reason is bounded like the API", () => {
  assert.equal(reasonTooLong("x".repeat(500)), false);
  assert.equal(reasonTooLong("x".repeat(501)), true);
  assert.equal(reasonTooLong(" ".repeat(600)), false, "whitespace is trimmed");
});

test("the dialog speaks about the kind of post and is destructive only for a takedown", () => {
  assert.deepEqual(dialogCopy({ action: "unpublish", kind: "reply" }), {
    title: "forum.visibility.unpublish_title_reply",
    body: "forum.visibility.unpublish_body_reply",
    action: "forum.visibility.unpublish",
    destructive: true,
  });
  assert.equal(
    dialogCopy({ action: "republish", kind: "topic" }).destructive,
    false,
  );
  assert.equal(
    dialogCopy({ action: "republish", kind: "topic" }).title,
    "forum.visibility.republish_title_topic",
  );
});

test("refusals map to the moderator's situation", () => {
  assert.equal(
    visibilityProblem(409, "unpublish"),
    "forum.toast.unpublish_conflict",
  );
  assert.equal(
    visibilityProblem(409, "republish"),
    "forum.toast.republish_conflict",
  );
  assert.equal(
    visibilityProblem(404, "republish"),
    "forum.toast.already_moderated",
  );
  assert.equal(
    visibilityProblem(500, "unpublish"),
    null,
    "other errors show the API text",
  );
});
