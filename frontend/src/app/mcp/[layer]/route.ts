import { proxyMcp } from "@/lib/server/mcp-proxy";

/** MCP / OAuth passthrough to the Go API (CF-173); see `mcp-proxy.ts`. */
export const dynamic = "force-dynamic";
/** Align with the API write timeout: an MCP answer may stream. */
export const maxDuration = 120;

const handle = (request: Request) => proxyMcp(request);

export const GET = handle;
export const POST = handle;
export const DELETE = handle;
export const OPTIONS = handle;
