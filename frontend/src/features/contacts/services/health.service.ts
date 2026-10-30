import type { components } from "@/generated/api";
import { getActiveOrganizationSlug } from "@/features/organizations/lib/active-organization-store";
import { apiClient, unwrap } from "@/lib/api";

export type CompanyHealth = components["schemas"]["CompanyHealth"];
export type CompanyHealthSummary =
  components["schemas"]["CompanyHealthSummary"];
export type CustomerHealthSettings =
  components["schemas"]["CustomerHealthSettings"];
export type CustomerHealthSettingsInput =
  components["schemas"]["CustomerHealthSettingsInput"];

function header() {
  return { "X-Organization": getActiveOrganizationSlug() ?? "" };
}

/** Customer health score (CF-87). */
export const healthService = {
  async get(companyUuid: string) {
    return unwrap<CompanyHealth>(
      await apiClient.GET("/v1/tenant/companies/{uuid}/health", {
        params: { header: header(), path: { uuid: companyUuid } },
      }),
    );
  },
  async recompute(companyUuid: string) {
    return unwrap<CompanyHealth>(
      await apiClient.POST("/v1/tenant/companies/{uuid}/health/recompute", {
        params: { header: header(), path: { uuid: companyUuid } },
      }),
    );
  },
  async settings() {
    return unwrap<CustomerHealthSettings>(
      await apiClient.GET("/v1/tenant/customer-health/settings", {
        params: { header: header() },
      }),
    );
  },
  async saveSettings(body: CustomerHealthSettingsInput) {
    return unwrap<CustomerHealthSettings>(
      await apiClient.PUT("/v1/tenant/customer-health/settings", {
        params: { header: header() },
        body,
      }),
    );
  },
};
