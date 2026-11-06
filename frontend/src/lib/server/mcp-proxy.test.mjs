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

const { apiOrigin, mcpUpstreamPath, MCP_REQUEST_HEADERS } = await load(
  "./mcp-proxy-core.ts",
);

test("forwards the MCP endpoint, the OAuth endpoints and discovery", () => {
  assert.equal(mcpUpstreamPath("/mcp/business", "POST"), "/mcp/business");
  assert.equal(mcpUpstreamPath("/mcp/business", "get"), "/mcp/business");
  assert.equal(mcpUpstreamPath("/mcp/business/", "DELETE"), "/mcp/business");
  assert.equal(mcpUpstreamPath("/oauth/token", "POST"), "/oauth/token");
  assert.equal(
    mcpUpstreamPath("/oauth/register", "OPTIONS"),
    "/oauth/register",
  );
  assert.equal(mcpUpstreamPath("/oauth/authorize", "GET"), "/oauth/authorize");
  assert.equal(
    mcpUpstreamPath("/.well-known/oauth-authorization-server", "GET"),
    "/.well-known/oauth-authorization-server",
  );
  assert.equal(
    mcpUpstreamPath(
      "/.well-known/oauth-protected-resource/mcp/business",
      "GET",
    ),
    "/.well-known/oauth-protected-resource/mcp/business",
  );
});

test("never forwards other paths, layers or methods", () => {
  for (const [path, method] of [
    ["/mcp/platform", "POST"],
    ["/mcp/customer", "POST"],
    ["/mcp/business/extra", "POST"],
    ["/mcp/../v1/auth/me", "GET"],
    ["/oauth/consent", "GET"],
    ["/oauth/authorize", "POST"],
    ["/oauth/token", "GET"],
    ["/oauth/internal", "POST"],
    ["/v1/auth/me", "GET"],
    ["/.well-known/oauth-protected-resource/mcp/platform", "GET"],
    ["/.well-known/openid-configuration", "GET"],
    ["/mcp/business", "PUT"],
  ]) {
    assert.equal(mcpUpstreamPath(path, method), null, `${method} ${path}`);
  }
});

test("the panel session cookie is never forwarded", () => {
  assert.equal(MCP_REQUEST_HEADERS.includes("cookie"), false);
  assert.equal(MCP_REQUEST_HEADERS.includes("authorization"), true);
});

test("apiOrigin strips the /v1 suffix of the upstream base", () => {
  assert.equal(apiOrigin("http://api:8080/v1"), "http://api:8080");
  assert.equal(apiOrigin("http://api:8080/v1/"), "http://api:8080");
  assert.equal(
    apiOrigin("https://api.example.test"),
    "https://api.example.test",
  );
});
