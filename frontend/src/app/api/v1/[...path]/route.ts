import * as Sentry from "@sentry/nextjs";

import { proxyToUpstream } from "@/lib/server/bff-proxy";
import {
  isUpstreamNetworkError,
  upstreamUnavailableResponse,
} from "@/lib/server/upstream-resilience";

type RouteContext = {
  params: Promise<{ path: string[] }>;
};

/**
 * Long-lived SSE streams need a raised
 * function timeout. Align with API `HTTP_WRITE_TIMEOUT` (e.g. 120s).
 */
export const maxDuration = 120;

async function handle(request: Request, context: RouteContext) {
  const { path } = await context.params;
  const route = (path ?? []).join("/");
  try {
    return await proxyToUpstream(path ?? [], request);
  } catch (err) {
    if (request.signal.aborted) {
      // Client went away mid-request: not a server fault.
      return new Response(null, { status: 499 });
    }
    if (isUpstreamNetworkError(err)) {
      // The API is restarting (deploy) or unreachable: expected and
      // transient, so a warning and a breadcrumb instead of a Sentry
      // exception (CLICKFIX-FRONT-END-3, -4).
      console.warn("[api/v1] upstream unavailable", request.method, route);
      Sentry.addBreadcrumb({
        category: "bff-proxy",
        level: "warning",
        message: "upstream unavailable",
        data: { method: request.method, route },
      });
      return upstreamUnavailableResponse();
    }
    console.error("[api/v1]", route, err);
    Sentry.captureException(err, {
      tags: {
        "http.route": "/api/v1/[...path]",
        "http.method": request.method,
      },
      fingerprint: ["bff-proxy", request.method],
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
