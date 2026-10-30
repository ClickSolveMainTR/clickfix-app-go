import type { components } from "@/generated/api";
import { getActiveOrganizationSlug } from "@/features/organizations";
import { apiClient, unwrap } from "@/lib/api";

export type SidebarApp = components["schemas"]["SidebarApp"];
export type SidebarAppInput = components["schemas"]["SidebarAppInput"];
export type SidebarAppLaunch = components["schemas"]["SidebarAppLaunch"];
export type SidebarAppActionInput =
  components["schemas"]["SidebarAppActionInput"];
export type SidebarAppPlacement = SidebarAppInput["placement"];
export type SidebarAppScope = SidebarAppInput["scopes"][number];

/** Tenant routes are scoped by the active organization. */
function orgHeader() {
  return { "X-Organization": getActiveOrganizationSlug() ?? "" };
}

/**
 * Sidebar iframe apps (CF-118). Discovery and launch need
 * tenant.conversations.read; registration needs tenant.settings.write and an
 * owner/admin membership. Launch and action calls are silent: the host shows
 * its own state and answers the iframe instead of raising global toasts.
 */
export const sidebarAppsService = {
  async list() {
    const data = await unwrap<{ items?: SidebarApp[] }>(
      await apiClient.GET("/v1/tenant/apps", {
        params: { header: orgHeader(), query: { limit: 100, offset: 0 } },
      }),
    );
    return data.items ?? [];
  },

  async create(body: SidebarAppInput) {
    return unwrap<SidebarApp>(
      await apiClient.POST("/v1/tenant/apps", {
        params: { header: orgHeader() },
        body,
      }),
    );
  },

  async update(uuid: string, body: SidebarAppInput) {
    return unwrap<SidebarApp>(
      await apiClient.PUT("/v1/tenant/apps/{uuid}", {
        params: { header: orgHeader(), path: { uuid } },
        body,
      }),
    );
  },

  async remove(uuid: string) {
    return unwrap(
      await apiClient.DELETE("/v1/tenant/apps/{uuid}", {
        params: { header: orgHeader(), path: { uuid } },
      }),
    );
  },

  /** Short-lived (120 s) context token for one iframe; never cached. */
  async launch(
    uuid: string,
    context: { conversation_uuid?: string; contact_uuid?: string },
  ) {
    return unwrap<SidebarAppLaunch>(
      await apiClient.POST("/v1/tenant/apps/{uuid}/launch", {
        params: { header: orgHeader(), path: { uuid } },
        body: context,
      }),
      { silent: true },
    );
  },

  /** Allowlisted action forwarded with the launch token and the host session. */
  async action(uuid: string, body: SidebarAppActionInput) {
    return unwrap<{ applied: boolean }>(
      await apiClient.POST("/v1/tenant/apps/{uuid}/actions", {
        params: { header: orgHeader(), path: { uuid } },
        body,
      }),
      { silent: true },
    );
  },
};
