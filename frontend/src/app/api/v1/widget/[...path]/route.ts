import { clientIpFromHeaders, fetchUpstream } from "@/lib/server/upstream";
import {
  isUpstreamNetworkError,
  proxyResponse,
} from "@/lib/server/upstream-resilience";

/**
 * Public web widget relay (CF-55): `https://<app>/api/v1/widget/*` → Go
 * `/v1/widget/*`. The widget SDK runs on customers' sites, so unlike the
 * session BFF (`/api/v1/[...path]`) this route:
 * - never reads or forwards the app session (cookies are ignored; the
 *   visitor token travels in `Authorization`),
 * - answers CORS for any origin without credentials (the per-widget origin
 *   allowlist is enforced by the API from `X-Widget-Origin`),
 * - forwards only the headers the widget API needs, including the mobile
 *   app identity pair (`X-ClickFix-App-Key`, `X-ClickFix-App-Package`,
 *   CF-201) that native apps send in place of a page origin. The pair is not
 *   CORS-allowed on purpose: a browser page always has an origin, and the
 *   API rejects an app key that arrives with one.
 */

type RouteContext = { params: Promise<{ path: string[] }> };

const ALLOWED_METHODS = "GET, POST, PATCH, OPTIONS";
const ALLOWED_HEADERS = "Authorization, Content-Type";
const MAX_BODY_BYTES = 11 * 1024 * 1024;

function corsHeaders(request: Request): Headers {
  const headers = new Headers({ Vary: "Origin" });
  const origin = request.headers.get("origin");
  if (origin) {
    headers.set("Access-Control-Allow-Origin", origin);
    headers.set("Access-Control-Allow-Methods", ALLOWED_METHODS);
    headers.set("Access-Control-Allow-Headers", ALLOWED_HEADERS);
    headers.set("Access-Control-Max-Age", "600");
  }
  return headers;
}

function json(request: Request, status: number, error: string): Response {
  const headers = corsHeaders(request);
  headers.set("Content-Type", "application/json");
  return new Response(
    JSON.stringify({ success: false, error: { code: error } }),
    {
      status,
      headers,
    },
  );
}

/** Origin of the embedding page: Origin header, else the Referer's origin. */
function pageOrigin(request: Request): string | null {
  const origin = request.headers.get("origin");
  if (origin && origin !== "null") return origin;
  const referer = request.headers.get("referer");
  if (!referer) return null;
  try {
    return new URL(referer).origin;
  } catch {
    return null;
  }
}

function validSegments(segments: string[]): boolean {
  return (
    segments.length >= 2 &&
    segments.every(
      (seg) =>
        seg !== "" && seg !== "." && seg !== ".." && !/[/\\%?#]/.test(seg),
    )
  );
}

async function handle(request: Request, context: RouteContext) {
  if (request.method === "OPTIONS") {
    return new Response(null, { status: 204, headers: corsHeaders(request) });
  }
  const { path } = await context.params;
  const segments = path ?? [];
  if (!validSegments(segments)) return json(request, 404, "NOT_FOUND");

  const declared = Number(request.headers.get("content-length") ?? "0");
  if (declared > MAX_BODY_BYTES)
    return json(request, 413, "WIDGET_FILE_TOO_LARGE");

  const headers = new Headers({ Accept: "application/json" });
  // User-Agent feeds device targeting; the edge country header is only
  // trusted by the API when AUTH_QR_TRUST_GEO_HEADERS is on (CF-150).
  for (const name of [
    "authorization",
    "content-type",
    "user-agent",
    "cf-ipcountry",
    "x-clickfix-app-key",
    "x-clickfix-app-package",
  ]) {
    const value = request.headers.get(name);
    if (value) headers.set(name, value);
  }
  const origin = pageOrigin(request);
  if (origin) headers.set("X-Widget-Origin", origin);
  const ip = clientIpFromHeaders(request.headers);
  if (ip) headers.set("X-Forwarded-For", ip);

  const body =
    request.method === "GET" || request.method === "HEAD"
      ? null
      : await request.arrayBuffer();
  if (body && body.byteLength > MAX_BODY_BYTES) {
    return json(request, 413, "WIDGET_FILE_TOO_LARGE");
  }

  const url = new URL(request.url);
  try {
    const result = await fetchUpstream(
      `widget/${segments.join("/")}${url.search}`,
      { method: request.method, headers, body },
    );
    const out = corsHeaders(request);
    for (const name of ["content-type", "retry-after", "x-request-id"]) {
      const value = result.headers.get(name);
      if (value) out.set(name, value);
    }
    out.set("Cache-Control", "no-store");
    return proxyResponse(result.body, { status: result.status, headers: out });
  } catch (err) {
    if (isUpstreamNetworkError(err)) {
      console.warn("[api/v1/widget] upstream unavailable", segments.join("/"));
    } else {
      console.error("[api/v1/widget]", segments.join("/"), err);
    }
    return json(request, 502, "UPSTREAM_UNAVAILABLE");
  }
}

export const GET = handle;
export const POST = handle;
export const PATCH = handle;
export const OPTIONS = handle;
