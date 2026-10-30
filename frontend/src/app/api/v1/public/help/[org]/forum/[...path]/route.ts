import { upstreamConfig } from "@/config/api";
import { brand } from "@/config/brand";
import { isCrossSiteMutation } from "@/lib/server/bff-proxy";
import { clientIpFromHeaders } from "@/lib/server/upstream";
import { proxyResponse } from "@/lib/server/upstream-resilience";

/**
 * Community forum relay (CF-78): `/api/v1/public/help/{org}/forum/*` → Go
 * `/v1/public/help/{org}/forum/*`. Unlike the session BFF this route:
 * - never reads or forwards the app session (forum identity is the signed
 *   widget visitor: `Authorization: Bearer <visitor token>` + `X-Widget-ID`),
 * - only accepts same-origin mutations from the help center pages,
 * - sends the app origin as `Origin`; the API trusts its own origin when it
 *   checks the widget's domain allowlist.
 */

type RouteContext = {
  params: Promise<{ org: string; path: string[] }>;
};

const MAX_BODY_BYTES = 64 * 1024;
const FORWARD = ["authorization", "content-type", "x-widget-id"];

function json(status: number, code: string): Response {
  return new Response(JSON.stringify({ success: false, error: { code } }), {
    status,
    headers: {
      "Content-Type": "application/json",
      "Cache-Control": "no-store",
    },
  });
}

function validSegment(seg: string): boolean {
  return seg !== "" && seg !== "." && seg !== ".." && !/[/\\%?#]/.test(seg);
}

function appOrigin(request: Request): string {
  const raw = process.env.SITE_URL || process.env.AUTH_URL || brand.siteUrl;
  try {
    return new URL(raw).origin;
  } catch {
    return new URL(request.url).origin;
  }
}

async function handle(request: Request, context: RouteContext) {
  const { org, path } = await context.params;
  const segments = path ?? [];
  if (
    !validSegment(org) ||
    segments.length === 0 ||
    !segments.every(validSegment)
  ) {
    return json(404, "NOT_FOUND");
  }
  if (isCrossSiteMutation(request)) {
    return json(403, "CROSS_SITE_REQUEST_BLOCKED");
  }
  const declared = Number(request.headers.get("content-length") ?? "0");
  if (declared > MAX_BODY_BYTES) return json(413, "PAYLOAD_TOO_LARGE");

  const headers = new Headers({ Accept: "application/json" });
  for (const name of FORWARD) {
    const value = request.headers.get(name);
    if (value) headers.set(name, value);
  }
  if (request.method !== "GET") headers.set("Origin", appOrigin(request));
  const ip = clientIpFromHeaders(request.headers);
  if (ip) headers.set("X-Forwarded-For", ip);

  const body = request.method === "GET" ? null : await request.arrayBuffer();
  if (body && body.byteLength > MAX_BODY_BYTES) {
    return json(413, "PAYLOAD_TOO_LARGE");
  }

  const url = new URL(request.url);
  const target = `${upstreamConfig.baseUrl.replace(/\/$/, "")}/public/help/${encodeURIComponent(org)}/forum/${segments.join("/")}${url.search}`;
  try {
    const res = await fetch(target, {
      method: request.method,
      headers,
      body,
      cache: "no-store",
      signal: AbortSignal.timeout(10_000),
    });
    const out = new Headers();
    for (const name of [
      "content-type",
      "cache-control",
      "retry-after",
      "x-request-id",
    ]) {
      const value = res.headers.get(name);
      if (value) out.set(name, value);
    }
    return proxyResponse(await res.arrayBuffer(), {
      status: res.status,
      headers: out,
    });
  } catch (err) {
    console.error("[api/v1/public/help/forum]", segments.join("/"), err);
    return json(502, "UPSTREAM_UNAVAILABLE");
  }
}

export const GET = handle;
export const POST = handle;
