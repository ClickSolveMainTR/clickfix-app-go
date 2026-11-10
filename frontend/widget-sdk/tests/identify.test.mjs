// CF-176: a failed identify is visible to the integrator without `debug`
// (console warning + "error" event), on the boot({ user }) path too, and
// never leaks user_hash.
import assert from "node:assert/strict";
import test from "node:test";
import { load } from "./load.mjs";

const { identifyFailure } = await load("../src/shared/identify.ts");
const { Widget } = await load("../src/runtime/widget.ts");

const HASH = "5ca1ab1e".repeat(8);
const USER = { user_id: "user-176", user_hash: HASH, email: "a@example.com" };

const apiError = (status, code, message = "") =>
  Object.assign(new Error(message), { status, code });

test("identity errors explain the cause and where to fix it", () => {
  const disabled = identifyFailure(apiError(403, "WIDGET_IDENTITY_DISABLED"));
  assert.equal(disabled.code, "WIDGET_IDENTITY_DISABLED");
  assert.match(disabled.message, /403 WIDGET_IDENTITY_DISABLED/);
  assert.match(disabled.message, /no identity secret/);
  assert.match(disabled.message, /Security \(Güvenlik\)/);

  const invalid = identifyFailure(apiError(401, "WIDGET_IDENTITY_INVALID"));
  assert.equal(invalid.code, "WIDGET_IDENTITY_INVALID");
  assert.match(invalid.message, /401 WIDGET_IDENTITY_INVALID/);
  assert.match(invalid.message, /does not match/);
  assert.notEqual(invalid.message, disabled.message);
});

test("other failures get a code and never echo the server text", () => {
  const limited = identifyFailure(
    apiError(429, "RATE_LIMITED", `echo ${HASH}`),
  );
  assert.deepEqual(limited, {
    code: "RATE_LIMITED",
    message: "identify failed (RATE_LIMITED): the user stays anonymous.",
  });
  assert.equal(identifyFailure(apiError(500, HASH)).code, "HTTP_500");
  assert.equal(
    identifyFailure(new TypeError("fetch failed")).code,
    "NETWORK_ERROR",
  );
  assert.equal(identifyFailure("boom").code, "IDENTIFY_FAILED");
});

/** A widget past bootstrap, with the network and the DOM stubbed out. */
function widget(opts, call) {
  globalThis.window ??= {
    addEventListener() {},
    removeEventListener() {},
  };
  const w = new Widget("widget-id", opts, "https://app.example.test");
  const requests = [];
  w.api = {
    token: "",
    call: async (method, path, body) => {
      requests.push([method, path, body]);
      return call(method, path, body);
    },
  };
  w.bootstrap = async () => {
    w.boot = {};
  };
  for (const stub of [
    "mount",
    "startRealtime",
    "schedulePoll",
    "watchPage",
    "render",
    "setVisitor",
  ])
    w[stub] = () => {};
  w.refresh = async () => {};
  w.readyPayload = () => ({});
  const errors = [];
  w.on("error", (e) => errors.push(e));
  return { w, errors, requests };
}

/** Runs fn with console.warn / console.info captured. */
async function captured(fn) {
  const lines = { warn: [], info: [] };
  const { warn, info } = console;
  console.warn = (...a) => lines.warn.push(a.join(" "));
  console.info = (...a) => lines.info.push(a.join(" "));
  try {
    await fn();
  } finally {
    console.warn = warn;
    console.info = info;
  }
  return lines;
}

const reject = (status, code) => async () => {
  throw apiError(status, code, "user_hash does not match user_id");
};

test("boot({ user }) on a widget without a secret warns and emits error", async () => {
  const { w, errors, requests } = widget(
    { user: USER },
    reject(403, "WIDGET_IDENTITY_DISABLED"),
  );
  const ready = [];
  w.on("ready", () => ready.push(true));
  const lines = await captured(() => w.start());

  assert.deepEqual(
    requests.map((r) => r[1]),
    ["/identify"],
  );
  assert.equal(lines.warn.length, 1);
  assert.match(
    lines.warn[0],
    /^\[ClickFix\] identify was rejected \(403 WIDGET_IDENTITY_DISABLED\)/,
  );
  assert.equal(lines.info.length, 0, "debug is off");
  assert.equal(errors.length, 1);
  assert.equal(errors[0].code, "WIDGET_IDENTITY_DISABLED");
  assert.equal(ready.length, 1, "the widget still starts, anonymously");
  assert.ok(!JSON.stringify([lines, errors]).includes(HASH));
});

test("ClickFix.identify() rejects, warns and emits error", async () => {
  const { w, errors } = widget({}, reject(403, "WIDGET_IDENTITY_DISABLED"));
  await captured(() => w.start());
  let thrown;
  const lines = await captured(async () => {
    thrown = await w.identify(USER).catch((e) => e);
  });
  assert.equal(thrown.code, "WIDGET_IDENTITY_DISABLED");
  assert.equal(lines.warn.length, 1);
  assert.match(lines.warn[0], /WIDGET_IDENTITY_DISABLED/);
  assert.equal(errors.length, 1);
  assert.ok(!JSON.stringify([lines, errors]).includes(HASH));
});

test("an invalid user_hash is explained separately", async () => {
  const { w, errors } = widget(
    { user: USER },
    reject(401, "WIDGET_IDENTITY_INVALID"),
  );
  const lines = await captured(() => w.start());
  assert.equal(lines.warn.length, 1);
  assert.match(lines.warn[0], /401 WIDGET_IDENTITY_INVALID/);
  assert.match(lines.warn[0], /does not match/);
  assert.equal(errors[0].code, "WIDGET_IDENTITY_INVALID");
  assert.ok(!JSON.stringify([lines, errors]).includes(HASH));
});

test("a verified identify stays quiet", async () => {
  const { w, errors } = widget({ user: USER }, async () => ({
    id: "v",
    token: "t",
    verified: true,
  }));
  const seen = [];
  w.on("identify", (e) => seen.push(e));
  const lines = await captured(() => w.start());
  assert.deepEqual(lines.warn, []);
  assert.deepEqual(errors, []);
  assert.deepEqual(seen, [{ verified: true, company_verified: false }]);
});

// CF-206: the company id was covered by user_hash; the API says so.
test("a company-signed identify reports company_verified", async () => {
  const { w, errors } = widget(
    { user: { ...USER, company: { id: "F-77", name: "Acme" } } },
    async () => ({
      id: "v",
      token: "t",
      verified: true,
      company_verified: true,
    }),
  );
  const seen = [];
  w.on("identify", (e) => seen.push(e));
  const lines = await captured(() => w.start());
  assert.deepEqual(lines.warn, []);
  assert.deepEqual(errors, []);
  assert.deepEqual(seen, [{ verified: true, company_verified: true }]);
});

// CF-206: company.id was sent but user_hash signed only user_id: the user
// is verified, the company was not bound by the id, and the integrator is
// told how to sign both, without the ids or the hash in the console.
test("an unsigned company id warns without leaking the request", async () => {
  const { w, errors } = widget(
    { user: { ...USER, company: { id: "F-77" } } },
    async () => ({
      id: "v",
      token: "t",
      verified: true,
      company_verified: false,
    }),
  );
  const seen = [];
  w.on("identify", (e) => seen.push(e));
  const lines = await captured(() => w.start());
  assert.equal(lines.warn.length, 1);
  assert.match(
    lines.warn[0],
    /company\.id was sent but user_hash signs only user_id/,
  );
  assert.match(lines.warn[0], /user_id \+ "\\n" \+ company\.id/);
  assert.deepEqual(errors, []);
  assert.deepEqual(seen, [{ verified: true, company_verified: false }]);
  assert.ok(!JSON.stringify(lines).includes(HASH));
  assert.ok(!JSON.stringify(lines).includes("F-77"));
  assert.ok(!JSON.stringify(lines).includes("user-176"));
});
