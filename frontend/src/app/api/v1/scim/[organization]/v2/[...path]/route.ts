import { upstreamConfig } from "@/config/api";
import { clientIpFromHeaders } from "@/lib/server/upstream";

/**
 * SCIM 2.0 (CF-128) through the BFF. The API advertises the SCIM base under
 * the public frontend origin + `/api`; identity providers authenticate with
 * their own organization SCIM bearer token, so unlike the generic proxy this
 * route forwards the caller's Authorization header and never the session.
 */
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
const SEGMENT = /^[A-Za-z0-9_-]+$/;

type RouteContext = {
  params: Promise<{ organization: string; path: string[] }>;
};

async function handle(request: Request, context: RouteContext) {
  const { organization, path } = await context.params;
  if (!UUID.test(organization) || !path.every((p) => SEGMENT.test(p))) {
    return Response.json({ detail: "Resource not found" }, { status: 404 });
  }
  const url = new URL(request.url);
  const headers = new Headers({
    Accept: request.headers.get("accept") ?? "application/scim+json",
  });
  const auth = request.headers.get("authorization");
  if (auth) headers.set("Authorization", auth);
  const contentType = request.headers.get("content-type");
  if (contentType) headers.set("Content-Type", contentType);
  const ip = clientIpFromHeaders(request.headers);
  if (ip) headers.set("X-Forwarded-For", ip);
  const hasBody = request.method !== "GET" && request.method !== "HEAD";
  const res = await fetch(
    `${upstreamConfig.baseUrl.replace(/\/$/, "")}/scim/${organization}/v2/${path.join("/")}${url.search}`,
    {
      method: request.method,
      headers,
      body: hasBody ? await request.arrayBuffer() : undefined,
      cache: "no-store",
    },
  );
  return new Response(res.status === 204 ? null : await res.arrayBuffer(), {
    status: res.status,
    headers: {
      "Content-Type":
        res.headers.get("content-type") ?? "application/scim+json",
      "Cache-Control": "no-store",
    },
  });
}

export const GET = handle;
export const POST = handle;
export const PUT = handle;
export const PATCH = handle;
export const DELETE = handle;
