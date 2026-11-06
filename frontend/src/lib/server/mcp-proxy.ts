import { upstreamConfig } from "@/config/api";

import { apiOrigin, forwardMcp } from "./mcp-proxy-core";

/**
 * Route-handler entry of the MCP / OAuth passthrough (CF-173): the listed
 * public paths go to the Go API unchanged. The rules live in
 * `mcp-proxy-core.ts` (`forwardMcp`). Whatever goes wrong, the answer is a
 * plain 500 without detail: nothing of the request (it may carry a token or a
 * code) is logged or reported.
 */
export async function proxyMcp(request: Request): Promise<Response> {
  try {
    return await forwardMcp(request, {
      origin: apiOrigin(upstreamConfig.baseUrl),
    });
  } catch {
    return new Response(
      JSON.stringify({
        error: "server_error",
        error_description: "Internal server error",
      }),
      {
        status: 500,
        headers: {
          "Content-Type": "application/json",
          "Cache-Control": "no-store",
        },
      },
    );
  }
}
