import type { components } from "@/generated/api";
import { getActiveOrganizationSlug } from "@/features/organizations";
import { apiClient, unwrap } from "@/lib/api";

export type Capacity = components["schemas"]["Capacity"];
export type AgentLoad = components["schemas"]["AgentLoad"];
export type RoutingSettings = components["schemas"]["RoutingSettings"];
export type RoutingRule = components["schemas"]["RoutingRule"];
export type RoutingRuleInput = components["schemas"]["RoutingRuleInput"];
export type RoutingEvent = components["schemas"]["RoutingEvent"];
export type CapacityLimits = components["schemas"]["CapacityLimits"];

function orgHeader() {
  return { "X-Organization": getActiveOrganizationSlug() ?? "" };
}

export const routingService = {
  async capacity() {
    return unwrap<Capacity>(
      await apiClient.GET("/v1/tenant/capacity", {
        params: { header: orgHeader() },
      }),
    );
  },
  async settings() {
    return unwrap<RoutingSettings>(
      await apiClient.GET("/v1/tenant/routing/settings", {
        params: { header: orgHeader() },
      }),
    );
  },
  async saveSettings(body: RoutingSettings) {
    return unwrap<RoutingSettings>(
      await apiClient.PUT("/v1/tenant/routing/settings", {
        params: { header: orgHeader() },
        body,
      }),
    );
  },
  async rules() {
    const data = await unwrap<{ items: RoutingRule[] }>(
      await apiClient.GET("/v1/tenant/routing/rules", {
        params: { header: orgHeader() },
      }),
    );
    return data.items;
  },
  async createRule(body: RoutingRuleInput) {
    return unwrap<RoutingRule>(
      await apiClient.POST("/v1/tenant/routing/rules", {
        params: { header: orgHeader() },
        body,
      }),
    );
  },
  async updateRule(uuid: string, body: RoutingRuleInput) {
    return unwrap<RoutingRule>(
      await apiClient.PATCH("/v1/tenant/routing/rules/{uuid}", {
        params: { header: orgHeader(), path: { uuid } },
        body,
      }),
    );
  },
  async removeRule(uuid: string) {
    return unwrap(
      await apiClient.DELETE("/v1/tenant/routing/rules/{uuid}", {
        params: { header: orgHeader(), path: { uuid } },
      }),
    );
  },
  async events() {
    const data = await unwrap<{ items: RoutingEvent[] }>(
      await apiClient.GET("/v1/tenant/routing/events", {
        params: { header: orgHeader() },
      }),
    );
    return data.items;
  },
  async limits() {
    return unwrap<CapacityLimits>(
      await apiClient.GET("/v1/tenant/capacity/limits", {
        params: { header: orgHeader() },
      }),
    );
  },
  async setLimits(userUuid: string | null, limits: Record<string, number>) {
    return unwrap<CapacityLimits>(
      await apiClient.PUT("/v1/tenant/capacity/limits", {
        params: { header: orgHeader() },
        body: { user_uuid: userUuid, limits },
      }),
    );
  },
};
