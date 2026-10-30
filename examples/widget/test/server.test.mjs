import assert from "node:assert/strict";
import { createHmac } from "node:crypto";
import test from "node:test";

process.env.CLICKFIX_IDENTITY_SECRET = "example-secret-0123456789abcdef";
process.env.CLICKFIX_WIDGET_ID = "widget-<id>";
const { server, userHash } = await import("../server.mjs");

test("user_hash is hex HMAC-SHA256 of the user id (ClickFix identify contract)", () => {
  assert.equal(userHash("s", "u-1"), createHmac("sha256", "s").update("u-1").digest("hex"));
  assert.match(userHash("s", "u-1"), /^[0-9a-f]{64}$/);
});

test("the page embeds the escaped widget id and the hash endpoint signs ids", async () => {
  await new Promise((r) => server.listen(0, r));
  const base = `http://127.0.0.1:${server.address().port}`;
  try {
    const html = await (await fetch(base + "/")).text();
    assert.ok(html.includes('data-widget-id="widget-&#60;id&#62;"'));
    const res = await fetch(base + "/user-hash", { method: "POST", body: JSON.stringify({ user_id: "42" }) });
    assert.equal((await res.json()).user_hash, userHash(process.env.CLICKFIX_IDENTITY_SECRET, "42"));
    assert.equal((await fetch(base + "/user-hash", { method: "POST", body: "{}" })).status, 400);
  } finally {
    server.close();
  }
});
