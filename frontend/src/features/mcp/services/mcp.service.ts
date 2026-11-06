import type { components } from "@/generated/api";
import { apiClient, unwrap } from "@/lib/api";
import { tenantRequest } from "@/lib/api/tenant-request";

/**
 * MCP connection (CF-173). The guide is organization-scoped
 * (`tenant.settings.read`, the MCP connect permission); the consent screen
 * and the connected apps belong to the signed-in member. No call here
 * returns or sends a token: MCP clients get theirs from the OAuth endpoints.
 */
export type McpGuide = components["schemas"]["MCPGuide"];

/** `GET /v1/mcp/consent/{uuid}` (`mcpauth/model`, `Consent`). */
export type McpConsent = {
  request_uuid: string;
  client_id: string;
  client_name: string;
  redirect_host: string;
  resource: string;
  resource_url: string;
  layer: string;
  scopes: string[];
  write_requested: boolean;
  organizations: {
    uuid: string;
    slug: string;
    name: string;
    tool_count: number | null;
    can_write: boolean;
  }[];
  expires_at: string;
};

/** `GET /v1/mcp/grants` row (`mcpauth/model`, `Grant`): one connected app. */
export type McpGrant = {
  uuid: string;
  client_id: string;
  client_name: string;
  resource: string;
  layer: string;
  scopes: string[];
  organization_uuid: string | null;
  organization_name: string | null;
  organization_slug: string | null;
  created_at: string;
  last_used_at: string | null;
};

type Page<T> = { items: T[]; total: number; limit: number; offset: number };

export const mcpService = {
  guide: (slug: string) =>
    tenantRequest<McpGuide>(slug, "GET", "/v1/tenant/mcp/guide", {
      silent: true,
    }),

  async grants(limit = 100, offset = 0) {
    return unwrap<Page<McpGrant>>(
      await apiClient.GET("/v1/mcp/grants", {
        params: { query: { limit, offset } as never },
      }),
      { silent: true },
    );
  },

  async revokeGrant(uuid: string) {
    return unwrap(
      await apiClient.DELETE("/v1/mcp/grants/{uuid}", {
        params: { path: { uuid } },
      }),
    );
  },

  async consent(uuid: string) {
    return unwrap<McpConsent>(
      await apiClient.GET("/v1/mcp/consent/{uuid}", {
        params: { path: { uuid } },
      }),
      { silent: true },
    );
  },

  /** Approve or deny; the answer says where the browser goes next. */
  async decide(
    uuid: string,
    body: {
      decision: "approve" | "deny";
      organization_uuid?: string;
      allow_write?: boolean;
    },
  ) {
    return unwrap<{ redirect_url: string }>(
      await apiClient.POST("/v1/mcp/consent/{uuid}/decide", {
        params: { path: { uuid } },
        body: body as never,
      }),
      { silent: true },
    );
  },
};

export const mcpKeys = {
  guide: (org: string) => ["tenant", org, "mcp", "guide"] as const,
  grants: (user: string) => ["mcp", "grants", user] as const,
  consent: (uuid: string) => ["mcp", "consent", uuid] as const,
};
