import { isForbiddenProxyPath } from "@/lib/server/bff-proxy";
import { proxyResponse } from "@/lib/server/upstream-resilience";
import { clientIpFromHeaders, fetchUpstream } from "@/lib/server/upstream";

/**
 * Mobile app passthrough: `/api/mobile/v1/*` → Go `/v1/*`.
 *
 * Unlike the panel BFF this proxy holds no session: the app keeps its own
 * token pair (from `/v1/auth/login`) and sends `Authorization: Bearer`. The
 * header is forwarded as is; no cookie is read or written, tokens are not
 * stripped from responses and there is no CSRF check (no ambient credential
 * to abuse).
 *
 * Cookies are dropped, never forwarded, so cookie and Bearer authentication
 * never mix on this path: only the explicit Authorization header counts.
 * (This used to refuse any request with a Cookie header with 400, but React
 * Native's fetch sends whatever the phone's cookie store holds for the host,
 * e.g. a cookie an edge proxy or CDN set on the domain, so a phone that once
 * received one failed every call from then on: CF-79.) The same paths as the
 * panel BFF are off limits (`internal/*`, the QR token exchange, the desktop
 * web view session), except the desktop code exchange: the desktop app is a
 * native client like the phone (CF-234).
 */

/** Request headers the app may send upstream. */
const FORWARD_REQUEST_HEADERS = [
  "accept",
  "accept-language",
  "authorization",
  "content-type",
  "idempotency-key",
  "user-agent",
  "x-app-version",
  "x-organization",
  "x-request-id",
] as const;

/** Response headers returned to the app. */
const FORWARD_RESPONSE_HEADERS = [
  "content-type",
  "content-disposition",
  "cache-control",
  "retry-after",
  "x-request-id",
] as const;

function jsonError(status: number, code: string, message: string): Response {
  return new Response(
    JSON.stringify({ success: false, error: { code, message } }),
    {
      status,
      headers: {
        "Content-Type": "application/json",
        "Cache-Control": "no-store",
      },
    },
  );
}

export function mobileUpstreamHeaders(request: Request): Headers {
  const out = new Headers();
  for (const key of FORWARD_REQUEST_HEADERS) {
    const value = request.headers.get(key);
    if (value) out.set(key, value);
  }
  if (!out.has("accept")) out.set("accept", "application/json");
  // Go rate limits and session rows key on the client IP.
  const clientIp = clientIpFromHeaders(request.headers);
  if (clientIp) out.set("X-Forwarded-For", clientIp);
  return out;
}

export async function proxyMobileToUpstream(
  pathSegments: string[],
  request: Request,
): Promise<Response> {
  if (
    pathSegments.length === 0 ||
    isForbiddenProxyPath(pathSegments, { nativeClient: true })
  ) {
    return jsonError(404, "NOT_FOUND", "Not found");
  }
  const url = new URL(request.url);
  const body =
    request.method === "GET" || request.method === "HEAD"
      ? null
      : await request.arrayBuffer();

  let result;
  try {
    result = await fetchUpstream(`${pathSegments.join("/")}${url.search}`, {
      method: request.method,
      headers: mobileUpstreamHeaders(request),
      body: body && body.byteLength > 0 ? body : null,
    });
  } catch {
    return jsonError(
      503,
      "SERVICE_UNAVAILABLE",
      "The service is temporarily unavailable. Try again shortly.",
    );
  }

  const headers = new Headers();
  for (const key of FORWARD_RESPONSE_HEADERS) {
    const value = result.headers.get(key);
    if (value) headers.set(key, value);
  }
  return proxyResponse(result.body, { status: result.status, headers });
}
