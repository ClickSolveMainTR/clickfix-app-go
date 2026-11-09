// CF-193: the assistant mode of the support widget runs only on the
// panel's own origin and talks to the same-origin tenant routes.
import assert from "node:assert/strict";
import test from "node:test";
import { load } from "./load.mjs";

const { assistantAllowed, assistantHref, AssistantClient, AssistantError } =
  await load("../src/runtime/assistant.ts");

const PANEL = "https://app.clickfix.example";

test("only on the panel origin with a valid organization", () => {
  const opts = { organization: "acme" };
  assert.equal(assistantAllowed(PANEL, PANEL, opts), true);
  assert.equal(
    assistantAllowed(PANEL, "https://shop.example.com", opts),
    false,
  );
  assert.equal(assistantAllowed(PANEL, PANEL, undefined), false);
  assert.equal(assistantAllowed(PANEL, PANEL, { organization: "../x" }), false);
  assert.equal(assistantAllowed(PANEL, PANEL, { organization: "" }), false);
});

test("page links stay inside the panel", () => {
  assert.equal(
    assistantHref({ route: "/settings/sla", target: "sla-business-hours" }),
    "/settings/sla?focus=sla-business-hours",
  );
  assert.equal(assistantHref({ route: "/channels" }), "/channels");
  assert.equal(
    assistantHref({ route: "/channels", target: "Bad Target" }),
    "/channels",
  );
  for (const route of [
    "https://evil.example",
    "//evil.example",
    "/\t/evil",
    "/a\\b",
    "/a?x=1",
    "javascript:alert(1)",
    "",
  ])
    assert.equal(assistantHref({ route }), null, route);
  assert.equal(assistantHref(null), null);
});

test("client: thread on the widget surface, then message, confirm, reject", async () => {
  const calls = [];
  const replies = [
    { status: 201, body: { data: { uuid: "t-1" } } },
    {
      status: 200,
      body: {
        data: {
          reply: {
            role: "assistant",
            content: "Tamam",
            actions: [{ id: "a1", title: "Sayfaya git", status: "proposed" }],
          },
        },
      },
    },
    {
      status: 200,
      body: {
        data: {
          role: "tool",
          content: "",
          actions: [{ id: "a1", title: "Sayfaya git", status: "done" }],
        },
      },
    },
    { status: 204, body: null },
  ];
  const fetchFn = async (url, init) => {
    calls.push({ url, init });
    const r = replies.shift();
    return {
      ok: r.status < 300,
      status: r.status,
      json: async () => r.body ?? Promise.reject(new Error("empty")),
    };
  };
  const c = new AssistantClient("acme", () => "tr", fetchFn);
  const reply = await c.send("SLA nerede?", "/inbox");
  assert.equal(reply.actions[0].id, "a1");
  assert.equal(calls[0].url, "/api/v1/tenant/assistant/threads?locale=tr");
  assert.deepEqual(JSON.parse(calls[0].init.body), {
    context: { surface: "widget", route: "/inbox" },
  });
  assert.equal(calls[0].init.credentials, "same-origin");
  assert.equal(calls[0].init.headers["X-Organization"], "acme");
  assert.equal(
    calls[1].url,
    "/api/v1/tenant/assistant/threads/t-1/messages?locale=tr",
  );
  const done = await c.confirm("a1");
  assert.equal(done.actions[0].status, "done");
  assert.equal(
    calls[2].url,
    "/api/v1/tenant/assistant/threads/t-1/actions/a1/confirm?locale=tr",
  );
  await c.reject("a1");
  assert.match(calls[3].url, /\/actions\/a1\/reject\?/);
  for (const call of calls)
    assert.ok(call.url.startsWith("/api/v1/tenant/assistant/"));
});

test("client: API errors carry the code, never the server text", async () => {
  const c = new AssistantClient(
    "acme",
    () => "en",
    async () => ({
      ok: false,
      status: 403,
      json: async () => ({
        error: { code: "TENANT_FORBIDDEN", message: "secret detail" },
      }),
    }),
  );
  await assert.rejects(
    c.send("hi", "/"),
    (e) =>
      e instanceof AssistantError &&
      e.status === 403 &&
      e.code === "TENANT_FORBIDDEN" &&
      !e.message.includes("secret"),
  );
});

test("capture: false turns co-browsing and replay off", async () => {
  const { Widget } = await load("../src/runtime/widget.ts");
  const off = new Widget("w", { capture: false }, PANEL, null);
  const on = new Widget("w", {}, PANEL, null);
  let asked = 0;
  for (const w of [off, on]) {
    w.capture.askCapture = () => asked++;
    w.api.call = async () => ({ cobrowse_enabled: true });
  }
  const ev = {
    type: "cobrowse.requested",
    conversation_uuid: "c",
    session: { kind: "cobrowse" },
  };
  off.onRealtime(ev);
  await new Promise((r) => setTimeout(r, 0));
  assert.equal(asked, 0);
  on.onRealtime(ev);
  await new Promise((r) => setTimeout(r, 0));
  assert.equal(asked, 1);
  assert.equal(await off.startReplay(), undefined);
});
