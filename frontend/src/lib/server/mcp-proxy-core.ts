/**
 * Which public MCP / OAuth paths the web origin forwards to the Go API
 * (CF-173). The API's OAuth issuer is the web origin (`FRONTEND_URL`), so MCP
 * clients look for these paths here: discovery under `/.well-known`, the
 * OAuth endpoints under `/oauth` and the MCP endpoint itself. Anything else
 * is not forwarded. Free of imports so it runs under `node --test`.
 */

/** MCP layers the API serves (`mcpauth/model`, `Resources`). */
const LAYERS = new Set(["business"]);

/** OAuth endpoints and the methods each accepts (`mcpauth/routes.go`). */
const OAUTH: Record<string, readonly string[]> = {
  authorize: ["GET"],
  register: ["POST", "OPTIONS"],
  token: ["POST", "OPTIONS"],
  revoke: ["POST", "OPTIONS"],
};

const MCP_METHODS = ["POST", "GET", "DELETE", "OPTIONS"];
const DISCOVERY_METHODS = ["GET", "OPTIONS"];

/**
 * The upstream path for a public request, or null when the path or the
 * method is not one of the MCP / OAuth endpoints.
 */
export function mcpUpstreamPath(
  pathname: string,
  method: string,
): string | null {
  const verb = method.toUpperCase();
  const path = pathname.replace(/\/+$/, "");
  if (path === "/.well-known/oauth-authorization-server")
    return DISCOVERY_METHODS.includes(verb) ? path : null;
  const resource =
    /^\/\.well-known\/oauth-protected-resource\/mcp\/([a-z]+)$/.exec(path);
  if (resource)
    return LAYERS.has(resource[1]!) && DISCOVERY_METHODS.includes(verb)
      ? path
      : null;
  const oauth = /^\/oauth\/([a-z]+)$/.exec(path);
  if (oauth) return OAUTH[oauth[1]!]?.includes(verb) ? path : null;
  const mcp = /^\/mcp\/([a-z]+)$/.exec(path);
  if (mcp)
    return LAYERS.has(mcp[1]!) && MCP_METHODS.includes(verb) ? path : null;
  return null;
}

/**
 * Request headers passed upstream. The client's own `Authorization` (its MCP
 * access token) is forwarded; cookies never are, so the panel session cannot
 * be used through these paths.
 */
export const MCP_REQUEST_HEADERS = [
  "accept",
  "accept-language",
  "authorization",
  "content-type",
  "last-event-id",
  "mcp-protocol-version",
  "mcp-session-id",
  "user-agent",
  // CORS preflight of browser-based MCP clients; the API answers it.
  "origin",
  "access-control-request-method",
  "access-control-request-headers",
] as const;

/** Response headers returned to the client. */
export const MCP_RESPONSE_HEADERS = [
  "content-type",
  "cache-control",
  "pragma",
  "location",
  "retry-after",
  "www-authenticate",
  "mcp-session-id",
  "x-content-type-options",
  "access-control-allow-origin",
  "access-control-allow-methods",
  "access-control-allow-headers",
  "access-control-expose-headers",
  "access-control-max-age",
  "vary",
] as const;

/** The API origin from its `/v1` base URL (these endpoints sit outside `/v1`). */
export function apiOrigin(v1Base: string): string {
  return v1Base.replace(/\/+$/, "").replace(/\/v1$/i, "");
}
