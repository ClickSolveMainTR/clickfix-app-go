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
const { sipTarget, engineConfigFrom, normalizeIceServers } =
  await load("./engine.ts");
const { clientCallEvent, callFailure } = await load("./state.ts");
test("Netsantral dial targets normalize Turkish numbers and preserve extensions", () => {
  assert.equal(
    sipTarget("+90 555 111 22 33", "pbx.test"),
    "sip:05551112233@pbx.test",
  );
  assert.equal(sipTarget("101", "pbx.test"), "sip:101@pbx.test");
  assert.equal(sipTarget("", "pbx.test"), null);
});
test("unconfigured phone stays disabled and ICE strings become objects", () => {
  assert.equal(engineConfigFrom(null), null);
  assert.equal(engineConfigFrom({ enabled: true }), null);
  assert.deepEqual(normalizeIceServers(["stun:example.test", "invalid"]), [
    { urls: "stun:example.test" },
  ]);
  const c = engineConfigFrom({
    enabled: true,
    sip_websocket_url: "wss://pbx.test",
    sip_domain: "pbx.test",
    sip_username: "fixture",
    sip_password: "fixture",
    ice_servers: [],
  });
  assert.ok(c.iceServers.length);
});
test("browser summaries measure talk time and preserve call direction", () => {
  const call = {
    direction: "incoming",
    remote: "05551112233",
    startedAt: 10000,
    phase: "ended",
    endReason: null,
  };
  const e = clientCallEvent(call, {}, 0, 35000);
  assert.equal(e.direction, "inbound");
  assert.equal(e.talk_seconds, 25);
  assert.equal(e.client_status, "answered");
  assert.equal(callFailure(call), null);
});
test("early Netsantral Q850 refusal is distinguishable from customer busy", () => {
  assert.equal(
    callFailure({
      phase: "ended",
      direction: "outgoing",
      startedAt: null,
      statusCode: 486,
      reasonHeader: "Q.850;cause=17",
    }).kind,
    "pbxRejected",
  );
});
