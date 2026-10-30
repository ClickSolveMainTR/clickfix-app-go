import { API_URL, BASE_URL_ORIGIN, expect, test } from "./fixtures";

type AuthorizationServer = {
  issuer: string;
  authorization_endpoint: string;
  token_endpoint: string;
  registration_endpoint: string;
  revocation_endpoint: string;
  code_challenge_methods_supported: string[];
  grant_types_supported: string[];
  scopes_supported: string[];
};

type ProtectedResource = {
  resource: string;
  authorization_servers: string[];
  scopes_supported?: string[];
};

// MCP OAuth 2.1 discovery (backend/internal/modules/mcpauth). The documents advertise the public
// frontend origin as issuer, so MCP clients resolve every endpoint there.
test.describe("MCP OAuth discovery", () => {
  test("authorization server metadata (RFC 8414)", async ({ request }) => {
    const res = await request.get(
      `${API_URL}/.well-known/oauth-authorization-server`,
    );
    expect(res.status()).toBe(200);
    expect(res.headers()["content-type"]).toContain("application/json");
    const meta = (await res.json()) as AuthorizationServer;
    expect(meta.issuer).toBe(BASE_URL_ORIGIN);
    for (const [field, path] of [
      ["authorization_endpoint", "/oauth/authorize"],
      ["token_endpoint", "/oauth/token"],
      ["registration_endpoint", "/oauth/register"],
      ["revocation_endpoint", "/oauth/revoke"],
    ] as const) {
      expect(meta[field], field).toBe(`${meta.issuer}${path}`);
    }
    expect(meta.code_challenge_methods_supported).toEqual(["S256"]);
    expect(meta.grant_types_supported).toEqual(
      expect.arrayContaining(["authorization_code", "refresh_token"]),
    );
    expect(meta.scopes_supported).toEqual(
      expect.arrayContaining(["mcp:read", "mcp:write"]),
    );
  });

  test("protected resource metadata (RFC 9728) for the business MCP layer", async ({
    request,
  }) => {
    const res = await request.get(
      `${API_URL}/.well-known/oauth-protected-resource/mcp/business`,
    );
    expect(res.status()).toBe(200);
    const meta = (await res.json()) as ProtectedResource;
    expect(meta.resource).toBe(`${BASE_URL_ORIGIN}/mcp/business`);
    expect(meta.authorization_servers).toEqual([BASE_URL_ORIGIN]);
    expect(meta.scopes_supported).toEqual(
      expect.arrayContaining(["mcp:read", "mcp:write"]),
    );
    // Only the business layer is published (mcpauth model.Resources).
    const unknown = await request.get(
      `${API_URL}/.well-known/oauth-protected-resource/mcp/unknown`,
    );
    expect(unknown.status()).toBe(404);
  });

  test("dynamic client registration rejects a body without redirect URIs", async ({
    request,
  }) => {
    const res = await request.post(`${API_URL}/oauth/register`, {
      data: { client_name: "E2E" },
    });
    expect(res.status()).toBe(400);
    expect(((await res.json()) as { error: string }).error).toBe(
      "invalid_redirect_uri",
    );
  });

  // The issuer is the public frontend origin, but the Next.js app does not proxy /.well-known/*
  // and /oauth/* to Go yet (mcpauth's package comment says it does). In production only the
  // frontend is public, so MCP clients cannot discover the server. Enable once the proxy exists.
  test.fixme("discovery is served on the public origin", async ({
    request,
  }) => {
    const res = await request.get("/.well-known/oauth-authorization-server");
    expect(res.status()).toBe(200);
    expect(((await res.json()) as AuthorizationServer).issuer).toBe(
      BASE_URL_ORIGIN,
    );
  });
});
