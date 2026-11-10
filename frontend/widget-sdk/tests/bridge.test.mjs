// CF-199: webview bridge contract (docs/webview-bridge.md). Inbound
// messages are validated and never echoed, every host transport is
// detected in a fixed order, parent frames need an explicit origin and
// command results reach the host only when the host asked (id).
import assert from "node:assert/strict";
import test from "node:test";
import { load } from "./load.mjs";

const {
  parseCommand,
  detectTransport,
  acceptsMessageEvent,
  createBridge,
  COMMAND_TYPES,
  FORWARDED_EVENTS,
  BRIDGE_VERSION,
  MAX_MESSAGE_BYTES,
} = await load("../src/shared/bridge.ts");

const HASH = "5ca1ab1e".repeat(8);

test("identify is accepted only with user_id and user_hash; extras are trimmed", () => {
  const ok = parseCommand(
    JSON.stringify({
      type: "clickfix:identify",
      id: "req-1",
      user: {
        user_id: "1001",
        user_hash: HASH,
        name: "Ayşe",
        company: { id: "F-77", name: "Acme", nope: 1 },
        attributes: { plan: "pro", seats: 5, nested: { x: 1 } },
        junk: true,
      },
    }),
  );
  assert.equal(ok.ok, true);
  assert.deepEqual(ok.command, {
    type: "clickfix:identify",
    id: "req-1",
    user: {
      user_id: "1001",
      user_hash: HASH,
      name: "Ayşe",
      company: { id: "F-77", name: "Acme" },
      attributes: { plan: "pro", seats: 5 },
    },
  });
  const missing = parseCommand({
    type: "clickfix:identify",
    id: "r",
    user: { user_id: "1" },
  });
  assert.equal(missing.ok, false);
  assert.equal(missing.code, "INVALID_MESSAGE");
  assert.equal(missing.id, "r");
  assert.doesNotMatch(missing.message, /1001|5ca1ab1e/);
});

test("every command type parses; unknown, oversized and malformed messages are rejected without echo", () => {
  for (const type of COMMAND_TYPES) {
    const extra =
      type === "clickfix:identify"
        ? { user: { user_id: "u", user_hash: HASH } }
        : type === "clickfix:track"
          ? { name: "viewed_order", properties: { id: 42 } }
          : type === "clickfix:update"
            ? { update: { locale: "en" } }
            : type === "clickfix:consent"
              ? { granted: true }
              : {};
    const r = parseCommand({ type, ...extra });
    assert.equal(r.ok, true, type);
  }
  assert.equal(parseCommand({ type: "clickfix:evil" }).code, "UNKNOWN_COMMAND");
  assert.equal(parseCommand({ type: "other:thing" }).code, "INVALID_MESSAGE");
  assert.equal(parseCommand("not json").code, "INVALID_JSON");
  assert.equal(parseCommand(42).code, "INVALID_MESSAGE");
  assert.equal(
    parseCommand("x".repeat(MAX_MESSAGE_BYTES + 1)).code,
    "TOO_LARGE",
  );
  assert.equal(
    parseCommand({ type: "clickfix:track" }).code,
    "INVALID_MESSAGE",
  );
  assert.equal(
    parseCommand({ type: "clickfix:consent", granted: "yes" }).code,
    "INVALID_MESSAGE",
  );
  const open = parseCommand({
    type: "clickfix:open",
    conversation: "c-1",
    view: "messages",
  });
  assert.deepEqual(open.command, {
    type: "clickfix:open",
    id: undefined,
    view: "messages",
    conversation: "c-1",
  });
});

test("transport order: React Native, iOS, Android, Flutter, then a parent frame with an explicit origin", () => {
  const sent = [];
  const handler = (tag) => ({
    postMessage(s) {
      sent.push([tag, s]);
    },
  });
  const rn = {
    ReactNativeWebView: handler("rn"),
    webkit: { messageHandlers: { ClickFix: handler("ios") } },
  };
  assert.equal(detectTransport(rn).name, "react-native");
  assert.equal(
    detectTransport({
      webkit: { messageHandlers: { ClickFix: handler("ios") } },
    }).name,
    "ios",
  );
  assert.equal(
    detectTransport({ ClickFixAndroid: handler("android") }).name,
    "android",
  );
  assert.equal(
    detectTransport({ ClickFixFlutter: handler("flutter") }).name,
    "flutter",
  );
  const t = detectTransport({ ClickFixFlutter: handler("flutter") });
  t.send({ type: "clickfix:link", url: "https://example.com" });
  assert.deepEqual(sent.at(-1), [
    "flutter",
    '{"type":"clickfix:link","url":"https://example.com"}',
  ]);

  const parentMessages = [];
  const parent = {
    postMessage(m, o) {
      parentMessages.push([m, o]);
    },
  };
  const framed = { parent };
  assert.equal(detectTransport(framed), null, "no origin: no frame transport");
  assert.equal(
    detectTransport(framed, "*"),
    null,
    "wildcard origin is refused",
  );
  assert.equal(
    detectTransport(framed, "https://app.example.com/path"),
    null,
    "origin only",
  );
  const frame = detectTransport(framed, "https://app.example.com");
  assert.equal(frame.name, "frame");
  frame.send({
    type: "clickfix:event",
    event: "unread",
    payload: { count: 1 },
  });
  assert.deepEqual(parentMessages[0], [
    { type: "clickfix:event", event: "unread", payload: { count: 1 } },
    "https://app.example.com",
  ]);
  const top = { postMessage() {} };
  top.parent = top;
  assert.equal(
    detectTransport(top, "https://app.example.com"),
    null,
    "top window has no parent",
  );
});

test("message events: parent frame only from the configured origin, native/self messages only when not framed", () => {
  const parent = {};
  const framed = { parent, location: { origin: "https://widget.example" } };
  assert.equal(
    acceptsMessageEvent(
      { origin: "https://host.example", source: parent },
      framed,
      "https://host.example",
    ),
    true,
  );
  assert.equal(
    acceptsMessageEvent(
      { origin: "https://evil.example", source: parent },
      framed,
      "https://host.example",
    ),
    false,
  );
  assert.equal(
    acceptsMessageEvent(
      { origin: "https://host.example", source: parent },
      framed,
    ),
    false,
  );
  assert.equal(
    acceptsMessageEvent(
      { origin: "https://host.example", source: parent },
      framed,
      "*",
    ),
    false,
  );
  assert.equal(
    acceptsMessageEvent({ origin: "", source: null }, framed),
    false,
    "framed page ignores self messages",
  );
  const top = { location: { origin: "https://widget.example" } };
  top.parent = top;
  assert.equal(acceptsMessageEvent({ origin: "", source: null }, top), true);
  assert.equal(
    acceptsMessageEvent({ origin: "https://widget.example", source: top }, top),
    true,
  );
  assert.equal(
    acceptsMessageEvent({ origin: "https://other.example", source: {} }, top),
    false,
  );
});

test("bridge runs commands, answers only when the host sent an id and hides error text", async () => {
  const out = [];
  const ran = [];
  const bridge = createBridge({
    send: (m) => out.push(m),
    run: async (cmd) => {
      ran.push(cmd.type);
      if (cmd.type === "clickfix:get-state") return { open: true, unread: 2 };
      if (cmd.type === "clickfix:identify")
        throw Object.assign(new Error(`secret ${HASH}`), {
          code: "WIDGET_IDENTITY_INVALID",
        });
      if (cmd.type === "clickfix:reset") throw new Error("boom");
    },
  });
  assert.equal(bridge.version, BRIDGE_VERSION);
  await bridge.receive(JSON.stringify({ type: "clickfix:close" }));
  assert.deepEqual(out, [], "no id: no result");
  await bridge.receive({ type: "clickfix:get-state", id: "s1" });
  assert.deepEqual(out.at(-1), {
    type: "clickfix:result",
    id: "s1",
    ok: true,
    data: { open: true, unread: 2 },
  });
  await bridge.receive({
    type: "clickfix:identify",
    id: "i1",
    user: { user_id: "1", user_hash: HASH },
  });
  assert.deepEqual(out.at(-1), {
    type: "clickfix:result",
    id: "i1",
    ok: false,
    error: {
      code: "WIDGET_IDENTITY_INVALID",
      message: "command failed (WIDGET_IDENTITY_INVALID)",
    },
  });
  assert.doesNotMatch(JSON.stringify(out), new RegExp(HASH));
  await bridge.receive({ type: "clickfix:reset", id: "r1" });
  assert.equal(out.at(-1).error.code, "COMMAND_FAILED");
  await bridge.receive({ type: "clickfix:nope", id: "n1" });
  assert.equal(out.at(-1).error.code, "UNKNOWN_COMMAND");
  await bridge.receive("{bad");
  assert.equal(out.length, 4, "unparseable message without id gets no reply");
  assert.deepEqual(ran, [
    "clickfix:close",
    "clickfix:get-state",
    "clickfix:identify",
    "clickfix:reset",
  ]);
});

test("ready, events and links are forwarded; unknown events are not", () => {
  const out = [];
  const bridge = createBridge({ send: (m) => out.push(m), run: () => {} });
  bridge.ready({
    protocol_version: 1,
    sdk_version: "2.1.0",
    widget_id: "w",
    visitor_id: "v",
  });
  assert.deepEqual(out[0], {
    type: "clickfix:ready",
    bridge_version: 1,
    protocol_version: 1,
    sdk_version: "2.1.0",
    widget_id: "w",
    visitor_id: "v",
  });
  for (const e of FORWARDED_EVENTS) bridge.forward(e, { e });
  assert.equal(out.length, 1 + FORWARDED_EVENTS.length);
  bridge.forward("internal:debug", {});
  assert.equal(out.length, 1 + FORWARDED_EVENTS.length);
  bridge.link("https://help.example/article");
  assert.deepEqual(out.at(-1), {
    type: "clickfix:link",
    url: "https://help.example/article",
  });
  assert.ok(
    FORWARDED_EVENTS.includes("message") &&
      FORWARDED_EVENTS.includes("unread") &&
      FORWARDED_EVENTS.includes("conversation:ended"),
  );
});
