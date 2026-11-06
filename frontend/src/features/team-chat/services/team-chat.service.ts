import type { components } from "@/generated/api";
import { getActiveOrganizationSlug } from "@/features/organizations";
import { apiClient, unwrap } from "@/lib/api";

export type Thread = components["schemas"]["InternalThread"];
export type ThreadInput = components["schemas"]["InternalThreadInput"];
export type ThreadMessage = components["schemas"]["InternalMessage"];

/** The org header is added by the client middleware; typed for openapi-fetch. */
function header() {
  return { "X-Organization": getActiveOrganizationSlug() ?? "" };
}

/**
 * Team chat (CF-106, web in CF-161): `/v1/tenant/team-chat`. Ported from
 * mobile/src/features/team/api.ts; same endpoints and page sizes.
 */
export const teamChatService = {
  async threads() {
    return unwrap<{ items: Thread[]; total: number }>(
      await apiClient.GET("/v1/tenant/team-chat", {
        params: { header: header(), query: { limit: 100 } },
      }),
    );
  },

  async create(body: ThreadInput) {
    return unwrap<Thread>(
      await apiClient.POST("/v1/tenant/team-chat", {
        params: { header: header() },
        body,
      }),
    );
  },

  /** The newest 100 messages, newest first (the API orders by id DESC). */
  async messages(uuid: string) {
    return unwrap<ThreadMessage[]>(
      await apiClient.GET("/v1/tenant/team-chat/{uuid}/messages", {
        params: { header: header(), path: { uuid }, query: { limit: 100 } },
      }),
    );
  },

  /** Mentioned members (thread members only) are notified by the backend. */
  async send(uuid: string, body: string, mentions: string[] = []) {
    return unwrap<ThreadMessage>(
      await apiClient.POST("/v1/tenant/team-chat/{uuid}/messages", {
        params: { header: header(), path: { uuid } },
        body: {
          body,
          client_id: crypto.randomUUID(),
          ...(mentions.length ? { mentions } : {}),
        },
      }),
    );
  },

  /** The answer is posted to the thread as an AI message and returned. */
  async askAi(uuid: string, question: string) {
    return unwrap<ThreadMessage>(
      await apiClient.POST("/v1/tenant/team-chat/{uuid}/ask-ai", {
        params: { header: header(), path: { uuid } },
        body: { question },
      }),
    );
  },

  async markRead(uuid: string) {
    return unwrap<{ read: boolean }>(
      await apiClient.POST("/v1/tenant/team-chat/{uuid}/read", {
        params: { header: header(), path: { uuid } },
      }),
    );
  },
};
