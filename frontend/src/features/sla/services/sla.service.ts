import type { components } from "@/generated/api";
import { getActiveOrganizationSlug } from "@/features/organizations";
import { apiClient, unwrap } from "@/lib/api";

export type SlaPolicy = components["schemas"]["SLAPolicy"];
export type SlaPolicyInput = components["schemas"]["SLAPolicyInput"];
export type SlaTarget = components["schemas"]["SLATarget"];
export type SlaBusinessHours = components["schemas"]["SLABusinessHours"];
export type SlaBusinessHoursInput =
  components["schemas"]["SLABusinessHoursInput"];
export type SlaWeek = components["schemas"]["SLAWeek"];
export type SlaHoliday = components["schemas"]["SLAHoliday"];
export type SlaHolidayInput = components["schemas"]["SLAHolidayInput"];
export type SlaPublicHoliday = components["schemas"]["SLAPublicHoliday"];
export type ConversationSla = components["schemas"]["ConversationSLA"];
export type SlaMetric = components["schemas"]["SLAMetric"];
export type SlaContractedCompany =
  components["schemas"]["SLAContractedCompany"];

/** Tenant routes are scoped by the active organization. */
function orgHeader() {
  return { "X-Organization": getActiveOrganizationSlug() ?? "" };
}

export const slaService = {
  async policies() {
    const data = await unwrap<{ items: SlaPolicy[] }>(
      await apiClient.GET("/v1/tenant/sla/policies", {
        params: { header: orgHeader() },
      }),
    );
    return data.items;
  },

  async createPolicy(body: SlaPolicyInput) {
    return unwrap<SlaPolicy>(
      await apiClient.POST("/v1/tenant/sla/policies", {
        params: { header: orgHeader() },
        body,
      }),
    );
  },

  async updatePolicy(uuid: string, body: SlaPolicyInput) {
    return unwrap<SlaPolicy>(
      await apiClient.PATCH("/v1/tenant/sla/policies/{uuid}", {
        params: { header: orgHeader(), path: { uuid } },
        body,
      }),
    );
  },

  async removePolicy(uuid: string) {
    return unwrap(
      await apiClient.DELETE("/v1/tenant/sla/policies/{uuid}", {
        params: { header: orgHeader(), path: { uuid } },
      }),
    );
  },

  async businessHours() {
    const data = await unwrap<{ items: SlaBusinessHours[] }>(
      await apiClient.GET("/v1/tenant/sla/business-hours", {
        params: { header: orgHeader() },
      }),
    );
    return data.items;
  },

  async createBusinessHours(body: SlaBusinessHoursInput) {
    return unwrap<SlaBusinessHours>(
      await apiClient.POST("/v1/tenant/sla/business-hours", {
        params: { header: orgHeader() },
        body,
      }),
    );
  },

  async updateBusinessHours(uuid: string, body: SlaBusinessHoursInput) {
    return unwrap<SlaBusinessHours>(
      await apiClient.PATCH("/v1/tenant/sla/business-hours/{uuid}", {
        params: { header: orgHeader(), path: { uuid } },
        body,
      }),
    );
  },

  async removeBusinessHours(uuid: string) {
    return unwrap(
      await apiClient.DELETE("/v1/tenant/sla/business-hours/{uuid}", {
        params: { header: orgHeader(), path: { uuid } },
      }),
    );
  },

  async holidays() {
    const data = await unwrap<{ items: SlaHoliday[] }>(
      await apiClient.GET("/v1/tenant/sla/holidays", {
        params: { header: orgHeader() },
      }),
    );
    return data.items;
  },

  async createHoliday(body: SlaHolidayInput) {
    return unwrap<SlaHoliday>(
      await apiClient.POST("/v1/tenant/sla/holidays", {
        params: { header: orgHeader() },
        body,
      }),
    );
  },

  async removeHoliday(uuid: string) {
    return unwrap(
      await apiClient.DELETE("/v1/tenant/sla/holidays/{uuid}", {
        params: { header: orgHeader(), path: { uuid } },
      }),
    );
  },

  async publicHolidays(year: number, country = "TR") {
    const data = await unwrap<{ items: SlaPublicHoliday[] }>(
      await apiClient.GET("/v1/tenant/sla/public-holidays", {
        params: { header: orgHeader(), query: { year, country } },
      }),
    );
    return data.items;
  },

  async conversation(uuid: string) {
    return unwrap<ConversationSla>(
      await apiClient.GET("/v1/tenant/conversations/{uuid}/sla", {
        params: { header: orgHeader(), path: { uuid } },
      }),
    );
  },

  async contractedCompanies() {
    const data = await unwrap<{ items: SlaContractedCompany[] }>(
      await apiClient.GET("/v1/tenant/sla/companies", {
        params: { header: orgHeader() },
      }),
    );
    return data.items;
  },

  async setCompanyPolicy(companyUuid: string, policyUuid: string | null) {
    return unwrap(
      await apiClient.PUT("/v1/tenant/sla/companies/{uuid}", {
        params: { header: orgHeader(), path: { uuid: companyUuid } },
        body: { policy_uuid: policyUuid },
      }),
    );
  },
};
