import type { ResourceMeta } from "@/features/io/types";
import type {
  ListTenantMembersParams,
  TenantMember,
  TenantMemberPage,
  TenantMemberPatchRequest,
  TenantSeats,
} from "@/features/tenant-members/types";
import { tenantRequest } from "@/lib/api/tenant-request";

const BASE = "/v1/tenant/members";

/** Members of one organization (`X-Organization`). */
export const tenantMembersService = {
  list(slug: string, params: ListTenantMembersParams) {
    return tenantRequest<TenantMemberPage>(slug, "GET", BASE, {
      query: params,
    });
  },

  /** Paid (billable) seats in use and the total member count. */
  seats(slug: string) {
    return tenantRequest<TenantSeats>(slug, "GET", "/v1/tenant/seats");
  },

  meta(slug: string) {
    return tenantRequest<ResourceMeta>(slug, "GET", `${BASE}/meta`);
  },

  get(slug: string, userUuid: string) {
    return tenantRequest<TenantMember>(slug, "GET", `${BASE}/${userUuid}`);
  },

  update(slug: string, userUuid: string, body: TenantMemberPatchRequest) {
    return tenantRequest<TenantMember>(slug, "PATCH", `${BASE}/${userUuid}`, {
      body,
    });
  },

  remove(slug: string, userUuid: string) {
    return tenantRequest<{ status: string }>(
      slug,
      "DELETE",
      `${BASE}/${userUuid}`,
    );
  },
};
