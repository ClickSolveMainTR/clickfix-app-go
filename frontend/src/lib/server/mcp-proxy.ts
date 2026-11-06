import { upstreamConfig } from "@/config/api";
import { clientIpFromHeaders } from "@/lib/server/upstream";

import { apiOrigin, forwardMcp } from "./mcp-proxy-core";

/**
 * Route-handler entry of the MCP / OAuth passthrough (CF-173): the listed
 * public paths go to the Go API unchanged. The rules live in
 * `mcp-proxy-core.ts` (`forwardMcp`).
 */
export function proxyMcp(request: Request): Promise<Response> {
  return forwardMcp(request, {
    origin: apiOrigin(upstreamConfig.baseUrl),
    clientIp: clientIpFromHeaders(request.headers),
  });
}
