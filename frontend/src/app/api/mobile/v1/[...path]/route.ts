import * as Sentry from "@sentry/nextjs";

import { proxyMobileToUpstream } from "@/lib/server/mobile-proxy";

type RouteContext = {
  params: Promise<{ path: string[] }>;
};

/**
 * Mobile app API: Bearer passthrough to Go `/v1/*` (see mobile-proxy.ts).
 * No session cookie is read or written here.
 */
async function handle(request: Request, context: RouteContext) {
  const { path } = await context.params;
  try {
    return await proxyMobileToUpstream(path ?? [], request);
  } catch (err) {
    console.error("[api/mobile]", (path ?? []).join("/"), err);
    Sentry.captureException(err, {
      tags: {
        "http.route": "/api/mobile/v1/[...path]",
        "http.method": request.method,
      },
      fingerprint: ["bff-mobile-proxy", request.method],
    });
    return Response.json(
      {
        success: false,
        error: { code: "INTERNAL_ERROR", message: "Internal server error" },
      },
      { status: 500 },
    );
  }
}

export const GET = handle;
export const POST = handle;
export const PUT = handle;
export const PATCH = handle;
export const DELETE = handle;
export const HEAD = handle;
export const OPTIONS = handle;
