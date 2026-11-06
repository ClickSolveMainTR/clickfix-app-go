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
  consentLoginPath,
  consentRequestId,
  consentView,
  decideBody,
  decideErrorKey,
  lifetime,
  safeClientRedirect,
  selectedOrganization,
  writeOffered,
} = await load("./consent.ts");

const ID = "5b0a7d0e-6f0a-4f0e-9a7e-2f4d6c1b9e11";
const org = (uuid, can_write = false) => ({
  uuid,
  name: uuid,
  tool_count: 3,
  can_write,
});
const base = {
  requestId: ID,
  bootstrapped: true,
  authenticated: true,
  loading: false,
  errorStatus: null,
  organizations: [org("a")],
};

test("only a UUID is a consent request", () => {
  assert.equal(consentRequestId(ID), ID);
  assert.equal(consentRequestId(ID.toUpperCase()), ID);
  assert.equal(consentRequestId(null), null);
  assert.equal(consentRequestId(""), null);
  assert.equal(consentRequestId("../../v1/auth/me"), null);
  assert.equal(consentRequestId(`${ID}?x=1`), null);
  assert.equal(consentRequestId("javascript:alert(1)"), null);
});

test("a signed-out visitor signs in and returns to the same request", () => {
  assert.equal(
    consentLoginPath("/login", ID),
    `/login?next=${encodeURIComponent(`/oauth/consent?request=${ID}`)}`,
  );
});

test("signed out: nothing is shown and nothing is loaded", () => {
  assert.equal(
    consentView({ ...base, authenticated: false, organizations: null }),
    "sign_in",
  );
  assert.equal(
    consentView({ ...base, bootstrapped: false, authenticated: false }),
    "loading",
  );
});

test("an invalid link is refused before anything else", () => {
  assert.equal(
    consentView({ ...base, requestId: null, authenticated: false }),
    "invalid",
  );
});

test("without an organization where the member may connect, approval is impossible", () => {
  assert.equal(consentView({ ...base, organizations: [] }), "no_organization");
  assert.equal(
    decideBody({
      decision: "approve",
      organization: undefined,
      writeRequested: true,
      allowWrite: true,
    }),
    null,
  );
});

test("load failures are told apart", () => {
  assert.equal(
    consentView({ ...base, errorStatus: 404, organizations: null }),
    "expired",
  );
  assert.equal(
    consentView({ ...base, errorStatus: 403, organizations: null }),
    "forbidden",
  );
  assert.equal(
    consentView({ ...base, errorStatus: 500, organizations: null }),
    "failed",
  );
  assert.equal(
    consentView({ ...base, errorStatus: 0, organizations: null }),
    "failed",
  );
  assert.equal(
    consentView({ ...base, loading: true, organizations: null }),
    "loading",
  );
  assert.equal(consentView(base), "ready");
});

test("the only organization is preselected; several need a choice", () => {
  assert.equal(selectedOrganization([org("a")], "").uuid, "a");
  assert.equal(selectedOrganization([org("a"), org("b")], ""), undefined);
  assert.equal(selectedOrganization([org("a"), org("b")], "b").uuid, "b");
  assert.equal(selectedOrganization([org("a"), org("b")], "zzz"), undefined);
});

test("write is granted only when requested, permitted there and ticked", () => {
  assert.equal(writeOffered(true, org("a", true)), true);
  assert.equal(writeOffered(false, org("a", true)), false);
  assert.equal(writeOffered(true, org("a", false)), false);
  const approve = (o, writeRequested, allowWrite) =>
    decideBody({
      decision: "approve",
      organization: o,
      writeRequested,
      allowWrite,
    });
  assert.deepEqual(approve(org("a", true), true, true), {
    decision: "approve",
    organization_uuid: "a",
    allow_write: true,
  });
  assert.equal(approve(org("a", true), true, false).allow_write, false);
  assert.equal(approve(org("a", false), true, true).allow_write, false);
  assert.equal(approve(org("a", true), false, true).allow_write, false);
});

test("deny sends nothing but the decision", () => {
  assert.deepEqual(
    decideBody({
      decision: "deny",
      organization: org("a", true),
      writeRequested: true,
      allowWrite: true,
    }),
    { decision: "deny" },
  );
});

test("a refused decision explains itself", () => {
  assert.equal(decideErrorKey(404), "mcp.consent.expired");
  assert.equal(decideErrorKey(403), "mcp.consent.forbidden");
  assert.equal(decideErrorKey(500), "mcp.consent.failed");
  assert.equal(decideErrorKey(null), "mcp.consent.failed");
});

test("only https and loopback http are followed after a decision", () => {
  assert.equal(
    safeClientRedirect(
      "https://claude.ai/api/mcp/auth_callback?code=c&state=s",
    ),
    "https://claude.ai/api/mcp/auth_callback?code=c&state=s",
  );
  assert.equal(
    safeClientRedirect("http://127.0.0.1:7777/cb?code=c"),
    "http://127.0.0.1:7777/cb?code=c",
  );
  assert.ok(safeClientRedirect("http://localhost:53682/cb"));
  assert.ok(safeClientRedirect("http://[::1]:9000/cb"));
  for (const bad of [
    "javascript:alert(1)",
    "data:text/html,<script>1</script>",
    "http://evil.example/cb",
    "http://127.0.0.1.evil.example/cb",
    "//evil.example/cb",
    "/relative",
    "https://user:pass@evil.example/",
    "ftp://example.com/",
    "",
    null,
    undefined,
  ]) {
    assert.equal(safeClientRedirect(bad), null, String(bad));
  }
});

test("lifetimes read as hours or days", () => {
  assert.deepEqual(lifetime(3600), { unit: "hours", count: 1 });
  assert.deepEqual(lifetime(30 * 24 * 3600), { unit: "days", count: 30 });
  assert.deepEqual(lifetime(1800), { unit: "hours", count: 1 });
  assert.equal(lifetime(0), null);
  assert.equal(lifetime(undefined), null);
});
