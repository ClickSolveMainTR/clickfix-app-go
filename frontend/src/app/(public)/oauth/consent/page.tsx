"use client";

import { Suspense } from "react";

import { McpConsentPage } from "@/features/mcp/components/mcp-consent-page";

/** OAuth consent screen of the MCP server; `?request=<uuid>` (CF-173). */
export default function McpConsentRoute() {
  return (
    <Suspense>
      <McpConsentPage />
    </Suspense>
  );
}
