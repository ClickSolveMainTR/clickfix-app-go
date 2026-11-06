import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import { transform } from "esbuild";

// integration-setup.ts has type-only imports, so it loads on its own.
const { code } = await transform(
  await readFile(new URL("./integration-setup.ts", import.meta.url), "utf8"),
  { loader: "ts", format: "esm" },
);
const {
  draftOf,
  inputOf,
  integrationErrorKey,
  integrationState,
  isProviderAuthorizeUrl,
  suggestedRedirectUri,
  validateDraft,
} = await import(
  `data:text/javascript;base64,${Buffer.from(code).toString("base64")}`
);

const CALLBACK = "/settings/engineering/callback/github";
const REDIRECT = `https://panel.example.com${CALLBACK}`;
const draft = (over = {}, config = {}) => {
  const base = draftOf(undefined, REDIRECT);
  return { ...base, ...over, config: { ...base.config, ...config } };
};
const apiError = (status, code, message, details = []) => ({
  status,
  code,
  message,
  details,
});

test("the redirect URI is suggested only on an HTTPS origin", () => {
  assert.equal(
    suggestedRedirectUri("https://panel.example.com", CALLBACK),
    REDIRECT,
  );
  // The API rejects plain HTTP, so local development starts with no value.
  assert.equal(suggestedRedirectUri("http://localhost:3000", CALLBACK), "");
  assert.equal(draftOf(undefined, "").config.redirect_uri, "");
  assert.deepEqual(
    validateDraft("github", draft({}, { repository: "a/b", redirect_uri: "" })),
    [],
  );
});

test("a saved redirect URI wins over the suggestion", () => {
  const saved = {
    uuid: "u",
    provider: "github",
    enabled: true,
    connected: false,
    webhook_configured: false,
    oauth_configured: false,
    config: {
      repository: "acme/app",
      redirect_uri: "https://other.example.com/cb",
    },
  };
  assert.equal(
    draftOf(saved, REDIRECT).config.redirect_uri,
    "https://other.example.com/cb",
  );
});

test("validateDraft requires each provider's own fields", () => {
  assert.deepEqual(validateDraft("github", draft()), ["repository"]);
  assert.deepEqual(validateDraft("linear", draft()), ["team_id"]);
  assert.deepEqual(validateDraft("jira", draft()), [
    "site_url",
    "cloud_id",
    "project_id",
  ]);
  assert.deepEqual(
    validateDraft(
      "jira",
      draft(
        {},
        {
          site_url: "https://acme.atlassian.net",
          cloud_id: "c",
          project_id: "SUP",
        },
      ),
    ),
    [],
  );
});

test("validateDraft checks shapes the API also checks", () => {
  assert.deepEqual(
    validateDraft("github", draft({}, { repository: "no-slash" })),
    ["repository"],
  );
  assert.deepEqual(
    validateDraft(
      "jira",
      draft(
        {},
        {
          site_url: "http://acme.atlassian.net",
          cloud_id: "c",
          project_id: "SUP",
        },
      ),
    ),
    ["site_url"],
  );
  assert.deepEqual(
    validateDraft(
      "github",
      draft(
        {},
        { repository: "a/b", redirect_uri: "http://panel.example.com/cb" },
      ),
    ),
    ["redirect_uri"],
  );
  assert.deepEqual(
    validateDraft(
      "github",
      draft(
        {},
        {
          repository: "a/b",
          redirect_uri: "https://user@panel.example.com/cb",
        },
      ),
    ),
    ["redirect_uri"],
  );
  assert.deepEqual(
    validateDraft(
      "github",
      draft({ webhookSecret: "short" }, { repository: "a/b" }),
    ),
    ["webhook_secret"],
  );
  assert.deepEqual(
    validateDraft(
      "github",
      draft({ webhookSecret: "0123456789abcdef" }, { repository: "a/b" }),
    ),
    [],
  );
});

test("inputOf sends the provider's whole config and only typed secrets", () => {
  const github = inputOf(
    "github",
    draft(
      { states: { done: "x" }, clientId: " id ", clientSecret: "" },
      { repository: " acme/app ", team_id: "stale" },
    ),
    true,
  );
  assert.deepEqual(github, {
    config: { redirect_uri: REDIRECT, repository: "acme/app" },
    enabled: true,
    credentials: { client_id: "id" },
  });

  // No secret typed: `credentials` is omitted, so stored values are kept.
  const linear = inputOf(
    "linear",
    draft({ states: { done: " s1 ", todo: "  " } }, { team_id: "t" }),
    false,
  );
  assert.deepEqual(linear, {
    config: {
      redirect_uri: REDIRECT,
      team_id: "t",
      project_id: "",
      states: { done: "s1" },
    },
    enabled: false,
  });

  const jira = inputOf(
    "jira",
    draft(
      { transitions: { done: "31" }, webhookSecret: "0123456789abcdef" },
      {
        site_url: "https://acme.atlassian.net",
        cloud_id: "c",
        project_id: "SUP",
      },
    ),
    true,
  );
  assert.deepEqual(jira.config.transitions, { done: "31" });
  assert.deepEqual(jira.config.states, {});
  assert.deepEqual(jira.credentials, { webhook_secret: "0123456789abcdef" });
});

test("only the provider's own consent host is followed", () => {
  const ok = "https://github.com/login/oauth/authorize?client_id=x&state=y";
  assert.equal(isProviderAuthorizeUrl("github", ok), true);
  assert.equal(isProviderAuthorizeUrl("linear", ok), false);
  assert.equal(
    isProviderAuthorizeUrl("linear", "https://linear.app/oauth/authorize?x=1"),
    true,
  );
  assert.equal(
    isProviderAuthorizeUrl("jira", "https://auth.atlassian.com/authorize?x=1"),
    true,
  );
  for (const bad of [
    undefined,
    "",
    "not a url",
    "http://github.com/login/oauth/authorize",
    "https://github.com.evil.example/login/oauth/authorize",
    "https://evil.example/?next=https://github.com/",
    "javascript:alert(1)",
  ]) {
    assert.equal(isProviderAuthorizeUrl("github", bad), false, String(bad));
  }
});

test("integrationErrorKey prefers the code and status over the text", () => {
  assert.equal(integrationErrorKey(new Error("network")), "unknown");
  assert.equal(integrationErrorKey(null), "unknown");
  assert.equal(
    integrationErrorKey(
      apiError(400, "INTEGRATION_UNAVAILABLE", "Integration unavailable"),
    ),
    "unavailable",
  );
  assert.equal(
    integrationErrorKey(apiError(403, "FORBIDDEN", "Missing permission")),
    "forbidden",
  );
  assert.equal(
    integrationErrorKey(apiError(404, "NOT_FOUND", "Not found")),
    "not_configured",
  );
  assert.equal(integrationErrorKey(apiError(500, "INTERNAL", "x")), "unknown");
});

test("integrationErrorKey maps the API's validation messages", () => {
  const invalid = (message) =>
    apiError(422, "VALIDATION_ERROR", "Validation failed", [
      { message: `invalid input: ${message}`, code: "invalid" },
    ]);
  for (const [message, key] of [
    ["repository must be owner/name", "repository"],
    ["cloud_id is required", "cloud_id"],
    ["redirect_uri must use HTTPS", "redirect_uri"],
    ["site_url must use HTTPS", "site_url"],
    ["webhook_secret must contain at least 16 bytes", "webhook_secret"],
    ["unknown local state mapping", "states"],
    [
      "OAuth client credentials and HTTPS redirect_uri are required",
      "oauth_not_ready",
    ],
    ["OAuth state expired or already used", "state_expired"],
    ["invalid OAuth state or code", "state_invalid"],
  ]) {
    assert.equal(integrationErrorKey(invalid(message)), key, message);
  }
  // A reworded message still gets a sensible generic text.
  assert.equal(integrationErrorKey(invalid("something new")), "invalid");
});

test("integrationState follows enabled and connected", () => {
  const of = (enabled, connected) => integrationState({ enabled, connected });
  assert.equal(integrationState(undefined), "not_configured");
  assert.equal(of(false, true), "disabled");
  assert.equal(of(true, false), "needs_auth");
  assert.equal(of(true, true), "connected");
});
