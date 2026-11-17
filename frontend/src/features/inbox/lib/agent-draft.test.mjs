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

const { pickAgentDraft } = await load("./agent-draft.ts");

const conv = {
  last_message_at: "2026-11-17T10:00:00Z",
  last_message_direction: "in",
};
const job = (
  status,
  result = { reply: " Kargonuz yolda. ", confidence: 0.9 },
) => ({
  uuid: "job-1",
  task: "reply",
  status,
  result,
  created_at: "2026-11-17T10:00:01Z",
});
const msg = (type, at, extra = {}) => ({
  direction: "out",
  private: false,
  created_at: at,
  sender: { type },
  ...extra,
});

test("a draft job is offered as a sourced draft", () => {
  assert.deepEqual(pickAgentDraft([job("draft")], conv), {
    uuid: "job-1",
    text: "Kargonuz yolda.",
    confidence: 0.9,
    unsourced: false,
  });
});

test("a handoff with model text is an unsourced suggestion (CF-246)", () => {
  const d = pickAgentDraft(
    [job("handoff", { reply: "Belki yarın", confidence: 0.3 })],
    conv,
  );
  assert.equal(d.unsourced, true);
  assert.equal(d.text, "Belki yarın");
});

test("a handoff without text, sent or running jobs show nothing", () => {
  assert.equal(
    pickAgentDraft([job("handoff", { summary: "s", reply: "" })], conv),
    null,
  );
  assert.equal(pickAgentDraft([job("handoff", { summary: "s" })], conv), null);
  assert.equal(pickAgentDraft([job("completed")], conv), null);
  assert.equal(pickAgentDraft([job("running", {})], conv), null);
  assert.equal(pickAgentDraft(undefined, conv), null);
  assert.equal(
    pickAgentDraft([{ ...job("draft"), task: "triage" }], conv),
    null,
  );
});

test("only the newest reply job counts", () => {
  const older = { ...job("draft"), uuid: "old" };
  assert.equal(pickAgentDraft([job("completed"), older], conv), null);
});

test("a teammate's public reply after the job hides it", () => {
  const later = "2026-11-17T10:00:05Z";
  assert.equal(
    pickAgentDraft([job("draft")], conv, [msg("agent", later)]),
    null,
  );
  // The handoff notice and private notes do not.
  assert.ok(pickAgentDraft([job("handoff")], conv, [msg("bot", later)]));
  assert.ok(
    pickAgentDraft([job("draft")], conv, [
      msg("agent", later, { private: true }),
    ]),
  );
  // A reply before the job does not either.
  assert.ok(
    pickAgentDraft([job("draft")], conv, [
      msg("agent", "2026-11-17T09:59:00Z"),
    ]),
  );
});

test("without loaded messages the conversation summary decides", () => {
  const answered = {
    last_message_at: "2026-11-17T10:00:05Z",
    last_message_direction: "out",
  };
  assert.equal(pickAgentDraft([job("draft")], answered), null);
});
