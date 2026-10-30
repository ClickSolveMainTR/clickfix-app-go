import type { components } from "@/generated/api";
import { getActiveOrganizationSlug } from "@/features/organizations/lib/active-organization-store";
import { apiClient, unwrap } from "@/lib/api";

export type CompanyRevenue = components["schemas"]["CompanyRevenue"];
export type CompanyRevenueChange =
  components["schemas"]["CompanyRevenueChange"];
export type CompanyRevenueInput = components["schemas"]["CompanyRevenueInput"];
export type CompanyRevenueDetail = {
  revenue: CompanyRevenue | null;
  history: CompanyRevenueChange[];
};

function header() {
  return { "X-Organization": getActiveOrganizationSlug() ?? "" };
}

/** Customer revenue data of a company (CF-86). */
export const revenueService = {
  async get(companyUuid: string) {
    return unwrap<CompanyRevenueDetail>(
      await apiClient.GET("/v1/tenant/companies/{uuid}/revenue", {
        params: { header: header(), path: { uuid: companyUuid } },
      }),
    );
  },
  async put(companyUuid: string, body: CompanyRevenueInput) {
    return unwrap<CompanyRevenue>(
      await apiClient.PUT("/v1/tenant/companies/{uuid}/revenue", {
        params: { header: header(), path: { uuid: companyUuid } },
        body,
      }),
    );
  },
  async clear(companyUuid: string) {
    return unwrap(
      await apiClient.DELETE("/v1/tenant/companies/{uuid}/revenue", {
        params: { header: header(), path: { uuid: companyUuid } },
      }),
    );
  },
};
