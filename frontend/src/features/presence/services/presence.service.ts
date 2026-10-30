import type { components } from "@/generated/api";
import { getActiveOrganizationSlug } from "@/features/organizations";
import { apiClient, unwrap } from "@/lib/api";

export type PresenceStatus = components["schemas"]["PresenceStatus"];
export type PresenceMember = components["schemas"]["PresenceMember"];
export type PresenceMe = components["schemas"]["PresenceMe"];

/** The org header is added by the client middleware; typed for openapi-fetch. */
function header() {
  return { "X-Organization": getActiveOrganizationSlug() ?? "" };
}

export const presenceService = {
  async list() {
    const data = await unwrap<{ items: PresenceMember[] }>(
      await apiClient.GET("/v1/tenant/presence", {
        params: { header: header() },
      }),
    );
    return data.items;
  },

  async heartbeat() {
    return unwrap<PresenceMe>(
      await apiClient.POST("/v1/tenant/presence/heartbeat", {
        params: { header: header() },
      }),
    );
  },

  /** Automatic away (CF-54): the agent is inactive (true) or back (false). */
  async idle(idle: boolean) {
    return unwrap<PresenceMe>(
      await apiClient.POST("/v1/tenant/presence/idle", {
        params: { header: header() },
        body: { idle },
      }),
    );
  },

  async setStatus(status: PresenceStatus) {
    return unwrap<PresenceMe>(
      await apiClient.PUT("/v1/tenant/presence/me", {
        params: { header: header() },
        body: { status },
      }),
    );
  },

  async typing(conversationUuid: string, state: "start" | "stop") {
    return unwrap<{ sent: boolean }>(
      await apiClient.POST("/v1/tenant/realtime/typing", {
        params: { header: header() },
        body: { conversation_uuid: conversationUuid, state },
      }),
    );
  },
};
