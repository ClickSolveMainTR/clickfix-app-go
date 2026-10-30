import type { components } from "@/generated/api";
import { tenantRequest } from "@/lib/api/tenant-request";

export type TenantContextData = components["schemas"]["TenantContext"];

export const tenantKeys = {
  all: ["tenant"] as const,
  context: (slug: string) => ["tenant", slug, "context"] as const,
};

export const tenantService = {
  /** Organization scope of `slug` for the signed-in user. */
  getContext(slug: string, signal?: AbortSignal) {
    return tenantRequest<TenantContextData>(slug, "GET", "/v1/tenant/context", {
      silent: true,
      signal,
    });
  },

  /** The caller leaves the organization (409 `LAST_ORGANIZATION_OWNER`). */
  leave(slug: string) {
    return tenantRequest<{ status: string }>(slug, "POST", "/v1/tenant/leave", {
      silent: true,
    });
  },
};
