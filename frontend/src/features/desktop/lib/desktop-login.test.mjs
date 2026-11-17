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

const {
  desktopLoginParams,
  desktopLoginReturnPath,
  desktopLoginSignInPath,
  desktopLoginView,
  safeDesktopCallback,
  desktopOSLabel,
} = await load("./desktop-login.ts");
const { isSameOriginPost, readDesktopSessionInput } = await load(
  "./desktop-session.ts",
);

const state = "state-0123456789abcdef";
const challenge = "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM";
const code = "A".repeat(43);

test("only well-formed state and S256 challenge are accepted", () => {
  assert.deepEqual(desktopLoginParams(state, challenge), { state, challenge });
  assert.equal(desktopLoginParams("short", challenge), null);
  assert.equal(desktopLoginParams(`${state}&x=1`, challenge), null);
  assert.equal(desktopLoginParams(state, "abc"), null);
  assert.equal(desktopLoginParams(null, null), null);
});

test("sign-in comes back to the same desktop request", () => {
  const params = { state, challenge };
  assert.equal(
    desktopLoginReturnPath(params),
    `/desktop/login?state=${state}&challenge=${challenge}`,
  );
  const path = desktopLoginSignInPath("/login", params);
  const next = new URL(path, "https://x.test").searchParams.get("next");
  assert.equal(next, desktopLoginReturnPath(params));
});

test("view follows session and request state", () => {
  const base = {
    params: { state, challenge },
    bootstrapped: true,
    authenticated: true,
    sent: false,
    errorStatus: null,
  };
  assert.equal(desktopLoginView({ ...base, params: null }), "invalid");
  assert.equal(desktopLoginView({ ...base, bootstrapped: false }), "loading");
  assert.equal(desktopLoginView({ ...base, authenticated: false }), "sign_in");
  assert.equal(desktopLoginView(base), "confirm");
  assert.equal(desktopLoginView({ ...base, sent: true }), "sent");
  assert.equal(desktopLoginView({ ...base, errorStatus: 403 }), "forbidden");
  assert.equal(desktopLoginView({ ...base, errorStatus: 500 }), "failed");
});

test("only the app callback for this state is followed", () => {
  const ok = `clickfix://auth/callback?code=${code}&state=${state}`;
  assert.equal(safeDesktopCallback(ok, state), ok);
  assert.equal(safeDesktopCallback(ok, "other-state-0123456789"), null);
  assert.equal(
    safeDesktopCallback(
      `https://evil.test/?code=${code}&state=${state}`,
      state,
    ),
    null,
  );
  assert.equal(
    safeDesktopCallback(
      `clickfix://auth/callbackx?code=${code}&state=${state}`,
      state,
    ),
    null,
  );
  assert.equal(
    safeDesktopCallback(
      `clickfix://auth/callback?code=short&state=${state}`,
      state,
    ),
    null,
  );
  assert.equal(safeDesktopCallback(`${ok}&token=x`, state), null);
  assert.equal(safeDesktopCallback(null, state), null);
});

test("desktop OS labels", () => {
  assert.equal(desktopOSLabel("macos"), "macOS");
  assert.equal(desktopOSLabel("windows"), "Windows");
  assert.equal(desktopOSLabel("ios"), null);
});

test("desktop session accepts only same-origin posts", () => {
  const h = (o) => new Headers(o);
  assert.equal(isSameOriginPost(h({ "sec-fetch-site": "same-origin" })), true);
  assert.equal(isSameOriginPost(h({ "sec-fetch-site": "cross-site" })), false);
  assert.equal(isSameOriginPost(h({ "sec-fetch-site": "none" })), false);
  assert.equal(
    isSameOriginPost(h({ origin: "https://app.test", host: "app.test" })),
    true,
  );
  assert.equal(
    isSameOriginPost(h({ origin: "https://evil.test", host: "app.test" })),
    false,
  );
  assert.equal(
    isSameOriginPost(h({ origin: "null", host: "app.test" })),
    false,
  );
  assert.equal(isSameOriginPost(h({ host: "app.test" })), false);
});

test("desktop session reads the token from body or header only", () => {
  const jwt = "aaa.bbb.ccc";
  assert.deepEqual(
    readDesktopSessionInput(
      null,
      "application/x-www-form-urlencoded",
      `access_token=${jwt}&next=%2Finbox`,
    ),
    { accessToken: jwt, next: "/inbox" },
  );
  assert.deepEqual(
    readDesktopSessionInput(
      null,
      "application/json; charset=utf-8",
      JSON.stringify({ access_token: jwt }),
    ),
    { accessToken: jwt, next: null },
  );
  assert.deepEqual(readDesktopSessionInput(`Bearer ${jwt}`, null, ""), {
    accessToken: jwt,
    next: null,
  });
  assert.equal(readDesktopSessionInput(null, "application/json", "{"), null);
  assert.equal(readDesktopSessionInput(null, "text/plain", jwt), null);
  assert.equal(readDesktopSessionInput("Bearer not-a-jwt", null, ""), null);
});
