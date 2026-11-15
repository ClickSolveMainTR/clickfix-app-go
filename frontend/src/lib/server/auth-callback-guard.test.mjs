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
  appleDisplayName,
  callbackErrorRedirect,
  callbackProviderId,
  rejectCallback,
  toGuardProvider,
} = await load("./auth-callback-guard.ts");

// Shapes as the Auth.js provider factories return them (CF-227). Values are
// placeholders, never real Apple tokens or identities.
const apple = {
  id: "apple",
  type: "oidc",
  checks: ["nonce", "state"],
  options: { clientId: "x" },
};
const github = { id: "github", type: "oauth", options: { clientId: "x" } };
const emailCode = {
  id: "credentials",
  type: "credentials",
  options: { id: "email-code" },
};
const providers = [apple, github, emailCode].map(toGuardProvider);

const form = (fields) => {
  const f = new FormData();
  for (const [k, v] of Object.entries(fields)) f.set(k, v);
  return f;
};
const post = (pathname, fields = {}) => ({
  method: "POST",
  pathname,
  query: new URLSearchParams(),
  form: form(fields),
});
const get = (pathname, query = "") => ({
  method: "GET",
  pathname,
  query: new URLSearchParams(query),
});

test("provider ids follow Auth.js: user options win", () => {
  assert.deepEqual(providers, [
    { id: "apple", type: "oidc", checks: ["nonce", "state"] },
    { id: "github", type: "oauth" },
    { id: "email-code", type: "credentials" },
  ]);
  assert.deepEqual(
    toGuardProvider(() => github),
    {
      id: "github",
      type: "oauth",
    },
  );
  assert.equal(toGuardProvider(null), null);
  assert.equal(toGuardProvider({ type: "oauth" }), null);
});

test("only callback paths are guarded", () => {
  assert.equal(callbackProviderId("/api/auth/session"), null);
  assert.equal(callbackProviderId("/api/auth/signin/apple"), null);
  assert.equal(callbackProviderId("/api/authx/callback"), null);
  assert.equal(callbackProviderId("/api/auth/callback/a/b"), null);
  assert.equal(callbackProviderId("/api/auth/callback"), "");
  assert.equal(callbackProviderId("/api/auth/callback/"), "");
  assert.equal(callbackProviderId("/api/auth/callback/undefined"), "");
  assert.equal(callbackProviderId("/api/auth/callback/apple"), "apple");
  assert.equal(rejectCallback(post("/api/auth/session"), providers), null);
});

test("POST /api/auth/callback without a provider is rejected (Sentry CLICKFIX-FRONT-END-8)", () => {
  assert.equal(
    rejectCallback(
      post("/api/auth/callback", { code: "c", state: "s" }),
      providers,
    ),
    "InvalidProvider",
  );
  assert.equal(
    rejectCallback(get("/api/auth/callback"), providers),
    "InvalidProvider",
  );
  assert.equal(
    rejectCallback(post("/api/auth/callback/undefined"), providers),
    "InvalidProvider",
  );
});

test("a callback for a provider that is not configured is rejected", () => {
  // e.g. Apple dropped from the list because its client secret expired
  assert.equal(
    rejectCallback(
      post("/api/auth/callback/apple", { code: "c", state: "s" }),
      [toGuardProvider(github)],
    ),
    "InvalidProvider",
  );
  assert.equal(
    rejectCallback(get("/api/auth/callback/google", "code=c"), providers),
    "InvalidProvider",
  );
});

test("Apple form_post: complete callbacks pass, first sign-in with the user field too", () => {
  assert.equal(
    rejectCallback(
      post("/api/auth/callback/apple", { code: "c", state: "s" }),
      providers,
    ),
    null,
  );
  assert.equal(
    rejectCallback(
      post("/api/auth/callback/apple", {
        code: "c",
        state: "s",
        user: '{"name":{"firstName":"A","lastName":"B"},"email":"a@example.invalid"}',
      }),
      providers,
    ),
    null,
  );
});

test("Apple form_post without code or state is rejected", () => {
  for (const fields of [
    { state: "s" },
    { code: "c" },
    { code: " ", state: "s" },
    {},
  ]) {
    assert.equal(
      rejectCallback(post("/api/auth/callback/apple", fields), providers),
      "OAuthCallbackError",
      JSON.stringify(fields),
    );
  }
  // a POST that is not a form at all
  assert.equal(
    rejectCallback(
      {
        method: "POST",
        pathname: "/api/auth/callback/apple",
        query: new URLSearchParams(),
        form: null,
      },
      providers,
    ),
    "OAuthCallbackError",
  );
  // the fields are read from the body, not the query string
  assert.equal(
    rejectCallback(
      {
        ...post("/api/auth/callback/apple"),
        query: new URLSearchParams("code=c&state=s"),
      },
      providers,
    ),
    "OAuthCallbackError",
  );
});

test("a provider-reported error (user cancelled) is left to Auth.js", () => {
  assert.equal(
    rejectCallback(
      post("/api/auth/callback/apple", {
        error: "user_cancelled_authorize",
        state: "s",
      }),
      providers,
    ),
    null,
  );
  assert.equal(
    rejectCallback(
      get("/api/auth/callback/github", "error=access_denied"),
      providers,
    ),
    null,
  );
});

test("query-string providers need code; state only when the provider checks it", () => {
  assert.equal(
    rejectCallback(get("/api/auth/callback/github", "code=c"), providers),
    null,
  );
  assert.equal(
    rejectCallback(get("/api/auth/callback/github", "state=s"), providers),
    "OAuthCallbackError",
  );
  assert.equal(
    rejectCallback(get("/api/auth/callback/github"), providers),
    "OAuthCallbackError",
  );
});

test("credentials callbacks are not checked for OAuth fields", () => {
  assert.equal(
    rejectCallback(
      post("/api/auth/callback/email-code", { email: "a@example.invalid" }),
      providers,
    ),
    null,
  );
});

test("the rejection is a relative 303 to the sign-in page with the error code", () => {
  const res = callbackErrorRedirect("/login", "InvalidProvider");
  assert.equal(res.status, 303);
  assert.equal(res.headers.get("location"), "/login?error=InvalidProvider");
  assert.equal(res.headers.get("cache-control"), "no-store");
});

test("Apple's display name survives a missing or partial user field", () => {
  assert.equal(appleDisplayName(undefined), null);
  assert.equal(appleDisplayName({}), null);
  assert.equal(appleDisplayName({ email: "a@example.invalid" }), null);
  assert.equal(appleDisplayName({ name: null }), null);
  assert.equal(appleDisplayName({ name: { firstName: "A" } }), "A");
  assert.equal(
    appleDisplayName({ name: { firstName: " ", lastName: "B" } }),
    "B",
  );
  assert.equal(
    appleDisplayName({ name: { firstName: "A", lastName: "B" } }),
    "A B",
  );
  assert.equal(
    appleDisplayName('{"name":{"firstName":"A","lastName":"B"}}'),
    "A B",
  );
  assert.equal(appleDisplayName("not json"), null);
});
