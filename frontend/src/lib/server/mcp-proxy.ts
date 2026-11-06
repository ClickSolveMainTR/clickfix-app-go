import { upstreamConfig } from "@/config/api";
import { clientIpFromHeaders } from "@/lib/server/upstream";
import { NULL_BODY_STATUSES } from "@/lib/server/upstream-resilience";

import {
  apiOrigin,
  MCP_REQUEST_HEADERS,
  MCP_RESPONSE_HEADERS,
  mcpUpstreamPath,
} from "./mcp-proxy-core";

/**
 * Public passthrough for the MCP server and its OAuth 2.1 authorization
 * server (CF-173): `/mcp/*`, `/oauth/*` and the two `/.well-known` discovery
 * documents go to the Go API unchanged. The API names the web origin as its
 * issuer, so this is where MCP clients connect.
 *
 * Like the mobile passthrough it holds no session: no cookie is read or
 * forwarded, and only the client's own `Authorization` header (its MCP access
 * token) counts. Redirects are returned to the client, never followed, so the
 * authorize step lands the browser on the consent screen. Bodies stream both
 * ways (the MCP endpoint may answer with an event stream).
 */
export async function proxyMcp(request: Request): Promise<Response> {
  const url = new URL(request.url);
  const path = mcpUpstreamPath(url.pathname, request.method);
  if (!path) return new Response(null, { status: 404 });

  const headers = new Headers();
  for (const key of MCP_REQUEST_HEADERS) {
    const value = request.headers.get(key);
    if (value) headers.set(key, value);
  }
  // The API's per-IP limits and audit trail key on the client address.
  const clientIp = clientIpFromHeaders(request.headers);
  if (clientIp) headers.set("X-Forwarded-For", clientIp);

  const hasBody = request.method !== "GET" && request.method !== "HEAD";
  let upstream: Response;
  try {
    upstream = await fetch(
      `${apiOrigin(upstreamConfig.baseUrl)}${path}${url.search}`,
      {
        method: request.method,
        headers,
        body: hasBody ? await request.arrayBuffer() : undefined,
        redirect: "manual",
        cache: "no-store",
        signal: request.signal,
      },
    );
  } catch {
    if (request.signal.aborted) return new Response(null, { status: 499 });
    return new Response(
      JSON.stringify({
        error: "temporarily_unavailable",
        error_description:
          "The service is temporarily unavailable. Try again shortly.",
      }),
      {
        status: 503,
        headers: {
          "Content-Type": "application/json",
          "Cache-Control": "no-store",
          "Retry-After": "5",
        },
      },
    );
  }

  const out = new Headers();
  for (const key of MCP_RESPONSE_HEADERS) {
    const value = upstream.headers.get(key);
    if (value) out.set(key, value);
  }
  return new Response(
    NULL_BODY_STATUSES.has(upstream.status) ? null : upstream.body,
    {
      status: upstream.status,
      headers: out,
    },
  );
}
