/**
 * Public passthrough for the MCP server and its OAuth 2.1 authorization
 * server (CF-173). The API names the web origin as its OAuth issuer
 * (`AUTH_FRONTEND_URL`), so MCP clients look for these paths on the web
 * origin: discovery under `/.well-known`, the OAuth endpoints under `/oauth`
 * and the MCP endpoint itself (`backend/docs/developer-platform.md`).
 *
 * Free of imports so the whole forwarding logic runs under `node --test`
 * against a real HTTP server.
 */

/** MCP layers the API serves (`mcpauth/model`, `Resources`). */
const LAYERS = new Set(["business"]);

/** OAuth endpoints and the methods each accepts (`mcpauth/routes.go`). */
const OAUTH = new Map<string, readonly string[]>([
  ["authorize", ["GET"]],
  ["register", ["POST", "OPTIONS"]],
  ["token", ["POST", "OPTIONS"]],
  ["revoke", ["POST", "OPTIONS"]],
]);

const MCP_METHODS = ["POST", "GET", "DELETE", "OPTIONS"];
const DISCOVERY_METHODS = ["GET", "OPTIONS"];

/**
 * The upstream path for a public request, or null when the path or the
 * method is not one of the MCP / OAuth endpoints. Every forwarded path is
 * spelled out here; there is no wildcard.
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
  // A Map, not an object: "/oauth/constructor" must not reach a prototype member.
  if (oauth) return OAUTH.get(oauth[1]!)?.includes(verb) ? path : null;
  const mcp = /^\/mcp\/([a-z]+)$/.exec(path);
  if (mcp)
    return LAYERS.has(mcp[1]!) && MCP_METHODS.includes(verb) ? path : null;
  return null;
}

/** The exact public surface, for documentation and tests. */
export const MCP_PUBLIC_PATHS = [
  "/.well-known/oauth-authorization-server",
  "/.well-known/oauth-protected-resource/mcp/business",
  "/oauth/authorize",
  "/oauth/register",
  "/oauth/token",
  "/oauth/revoke",
  "/mcp/business",
] as const;

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

/** Response headers returned to the client. `Set-Cookie` is never passed on. */
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

/** Largest request body forwarded (JSON-RPC calls and OAuth forms are small). */
export const MCP_MAX_BODY_BYTES = 1024 * 1024;

/**
 * How long the API may take to start answering. It covers connecting and the
 * response headers only: once the answer has started, an event stream may
 * stay open as long as the client keeps it.
 */
export const MCP_UPSTREAM_TIMEOUT_MS = 30_000;

/**
 * Best-effort client address for the API's per-address limits: the edge
 * proxy's `X-Real-IP`, else the right-most `X-Forwarded-For` hop (appended by
 * our own proxy; the left-most entries are whatever the client sent). Same
 * rule as `clientIpFromHeaders` (`upstream.ts`).
 */
export function mcpClientIp(headers: Headers): string | null {
  const real = headers.get("x-real-ip")?.trim();
  if (real) return real;
  const hops = (headers.get("x-forwarded-for") ?? "")
    .split(",")
    .map((hop) => hop.trim())
    .filter(Boolean);
  return hops.length > 0 ? hops[hops.length - 1]! : null;
}

const NO_BODY = new Set([204, 205, 304]);

/** The API origin from its `/v1` base URL (these endpoints sit outside `/v1`). */
export function apiOrigin(v1Base: string): string {
  return v1Base.replace(/\/+$/, "").replace(/\/v1$/i, "");
}

function problem(status: number, error: string, description: string) {
  return new Response(
    JSON.stringify({ error, error_description: description }),
    {
      status,
      headers: {
        "Content-Type": "application/json",
        "Cache-Control": "no-store",
      },
    },
  );
}

/**
 * Reads the request body up to `max` bytes; null when it is larger. Throws
 * when the client goes away mid-body (the caller answers without forwarding).
 */
async function readCapped(
  request: Request,
  max: number,
): Promise<Uint8Array | null> {
  const declared = Number(request.headers.get("content-length") ?? "0");
  if (Number.isFinite(declared) && declared > max) return null;
  if (!request.body) return new Uint8Array();
  const reader = request.body.getReader();
  const chunks: Uint8Array[] = [];
  let size = 0;
  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;
    size += value.byteLength;
    if (size > max) {
      await reader.cancel().catch(() => undefined);
      return null;
    }
    chunks.push(value);
  }
  const out = new Uint8Array(size);
  let at = 0;
  for (const chunk of chunks) {
    out.set(chunk, at);
    at += chunk.byteLength;
  }
  return out;
}

/**
 * Forwards one public MCP / OAuth request to the API at `origin`.
 *
 * - Only the listed paths and methods; anything else is 404 and never leaves.
 * - No session: cookies are not read or forwarded; only the client's own
 *   `Authorization` header counts.
 * - The body is forwarded unchanged, up to `MCP_MAX_BODY_BYTES` (413 above).
 * - Redirects are returned to the client, never followed, and the target is
 *   the API's, unchanged: the authorize step lands the browser on the consent
 *   screen, the consent decision sends it back to the client.
 * - Nothing is cached: a response without `Cache-Control` gets `no-store`.
 * - The API gets `MCP_UPSTREAM_TIMEOUT_MS` to start answering (504 after).
 * - Nothing is logged: tokens, codes and request ids pass through unread.
 * - It never throws: a client that goes away, an unreachable API and any
 *   other failure each have their own plain answer.
 */
export async function forwardMcp(
  request: Request,
  options: { origin: string; timeoutMs?: number },
): Promise<Response> {
  const url = new URL(request.url);
  const path = mcpUpstreamPath(url.pathname, request.method);
  if (!path) return new Response(null, { status: 404 });

  const headers = new Headers();
  for (const key of MCP_REQUEST_HEADERS) {
    const value = request.headers.get(key);
    if (value) headers.set(key, value);
  }
  // The API's per-IP limits and audit trail key on the client address. Only
  // one address is sent, so the API never reads a hop the client invented.
  const clientIp = mcpClientIp(request.headers);
  if (clientIp) headers.set("X-Forwarded-For", clientIp);

  let body: Uint8Array | undefined;
  if (request.method !== "GET" && request.method !== "HEAD") {
    let read: Uint8Array | null;
    try {
      read = await readCapped(request, MCP_MAX_BODY_BYTES);
    } catch {
      // The client went away before the whole body arrived.
      return new Response(null, { status: 499 });
    }
    if (read === null)
      return problem(413, "invalid_request", "The request body is too large.");
    body = read;
  }

  // Aborts when the client leaves, or when the API has not started answering
  // in time; the timer stops as soon as the response headers are here.
  const controller = new AbortController();
  let timedOut = false;
  const onClientAbort = () => controller.abort();
  request.signal.addEventListener("abort", onClientAbort);
  if (request.signal.aborted) controller.abort();
  const timer = setTimeout(() => {
    timedOut = true;
    controller.abort();
  }, options.timeoutMs ?? MCP_UPSTREAM_TIMEOUT_MS);

  let upstream: Response;
  try {
    upstream = await fetch(`${options.origin}${path}${url.search}`, {
      method: request.method,
      headers,
      body: body as BodyInit | undefined,
      redirect: "manual",
      cache: "no-store",
      signal: controller.signal,
    });
  } catch {
    request.signal.removeEventListener("abort", onClientAbort);
    if (timedOut)
      return problem(
        504,
        "temporarily_unavailable",
        "The service took too long to answer. Try again shortly.",
      );
    if (request.signal.aborted) return new Response(null, { status: 499 });
    const out = problem(
      503,
      "temporarily_unavailable",
      "The service is temporarily unavailable. Try again shortly.",
    );
    out.headers.set("Retry-After", "5");
    return out;
  } finally {
    clearTimeout(timer);
  }

  const out = new Headers();
  for (const key of MCP_RESPONSE_HEADERS) {
    const value = upstream.headers.get(key);
    if (value) out.set(key, value);
  }
  if (!out.has("cache-control")) out.set("Cache-Control", "no-store");
  return new Response(NO_BODY.has(upstream.status) ? null : upstream.body, {
    status: upstream.status,
    headers: out,
  });
}
