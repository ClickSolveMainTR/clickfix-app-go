import assert from "node:assert/strict";
import { test } from "node:test";

import {
  DecisionQueue,
  PROPOSE_TOOL,
  actionPart,
  toThreadMessage,
} from "./messages.ts";

const action = (status, extra = {}) => ({
  id: "act-1",
  kind: "channels.api.create",
  title: "REST API kanalı kur",
  summary: "",
  params: { name: "Shop" },
  status,
  ...extra,
});

test("a proposed card pauses the thread on an approval gate", () => {
  const m = toThreadMessage({
    uuid: "m1",
    role: "assistant",
    content: "Hazır.",
    created_at: "2026-01-01T00:00:00Z",
    actions: [action("proposed")],
    suggestions: ["Devam"],
  });
  assert.equal(m.role, "assistant");
  assert.deepEqual(m.status, { type: "requires-action", reason: "tool-calls" });
  assert.deepEqual(m.metadata.custom, {
    uuid: "m1",
    role: "assistant",
    suggestions: ["Devam"],
  });
  const [text, tool] = m.content;
  assert.deepEqual(text, { type: "text", text: "Hazır." });
  assert.equal(tool.toolName, PROPOSE_TOOL);
  assert.equal(tool.toolCallId, "act-1");
  assert.deepEqual(tool.approval, { id: "act-1" });
  assert.equal(tool.result, undefined);
  assert.equal(tool.artifact.status, "proposed");
});

test("answered cards are complete, with the decision recorded on the gate", () => {
  const done = actionPart(action("done", { result: "ok" }));
  assert.deepEqual(done.approval, { id: "act-1", approved: true });
  assert.equal(done.result.action.status, "done");
  assert.equal(done.isError, false);
  const failed = actionPart(action("failed", { result: "nope" }));
  assert.equal(failed.isError, true);
  const rejected = actionPart(action("rejected"));
  assert.deepEqual(rejected.approval, { id: "act-1", approved: false });
  const m = toThreadMessage({
    uuid: "m2",
    role: "assistant",
    content: "",
    created_at: "2026-01-01T00:00:00Z",
    actions: [action("done")],
  });
  assert.deepEqual(m.status, { type: "complete", reason: "stop" });
  assert.equal(m.content.length, 1);
});

test("user and tool notes keep their roles", () => {
  const user = toThreadMessage({
    uuid: "u1",
    role: "user",
    content: "Merhaba",
    created_at: "2026-01-01T00:00:00Z",
  });
  assert.equal(user.role, "user");
  assert.equal(user.content, "Merhaba");
  const note = toThreadMessage({
    uuid: "t1",
    role: "tool",
    content: "Destek ekibine iletildi.",
    created_at: "2026-01-01T00:00:00Z",
    interrupted: true,
  });
  assert.equal(note.role, "assistant");
  assert.equal(note.metadata.custom.role, "tool");
  assert.equal(note.metadata.custom.interrupted, true);
});

test("the decision queue hands every waiting decision over once", () => {
  const q = new DecisionQueue();
  q.push({ actionId: "a", approved: true, params: { name: "X" } });
  q.push({ actionId: "b", approved: false });
  q.push({ actionId: "a", approved: true });
  assert.equal(q.size, 2);
  assert.deepEqual(q.drain(), [
    { actionId: "a", approved: true },
    { actionId: "b", approved: false },
  ]);
  assert.equal(q.size, 0);
});
