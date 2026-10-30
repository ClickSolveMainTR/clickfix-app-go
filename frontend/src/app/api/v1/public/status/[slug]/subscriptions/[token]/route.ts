import { proxyToUpstream } from "@/lib/server/bff-proxy";

type RouteContext = {
  params: Promise<{ slug: string; token: string }>;
};

function segments(slug: string, token: string) {
  return ["public", "status", slug, "subscriptions", token];
}

/**
 * Status page e-mail links sent before CF-157 point here. A browser opening
 * one lands on the confirmation page (nothing changes until its button is
 * pressed); API calls (JSON) are proxied as usual.
 */
export async function GET(request: Request, context: RouteContext) {
  const { slug, token } = await context.params;
  const accept = request.headers.get("accept") ?? "";
  if (accept.includes("text/html")) {
    // A relative Location keeps the browser on the public origin even when
    // request.url carries the internal host behind the reverse proxy.
    const target = `/status/${encodeURIComponent(slug)}/subscription/${encodeURIComponent(token)}`;
    return new Response(null, {
      status: 303,
      headers: { Location: target, "Cache-Control": "no-store" },
    });
  }
  return proxyToUpstream(segments(slug, token), request);
}

export async function POST(request: Request, context: RouteContext) {
  const { slug, token } = await context.params;
  return proxyToUpstream(segments(slug, token), request);
}
