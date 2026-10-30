import type { components } from "@/generated/api";
import { getActiveOrganizationSlug } from "@/features/organizations";
import { apiClient, unwrap } from "@/lib/api";

export type ForumCategory = components["schemas"]["ForumCategory"];
export type ForumTopicSummary = components["schemas"]["ForumTopicSummary"];
export type ForumTopic = components["schemas"]["ForumTopic"];
export type ForumReply = components["schemas"]["ForumReply"];
export type ForumConversion = components["schemas"]["ForumConversion"];
export type ForumSubmitted = components["schemas"]["ForumSubmitted"];

/**
 * A pending post of the moderation queue. The OpenAPI item is an open
 * object (the backend returns the stored forum post); only the fields the
 * queue renders are read. `topic_id` is null for topics, set for replies.
 */
export type ForumModerationItem = {
  uuid: string;
  slug: string;
  title: string;
  body: string;
  status: string;
  flag_reason: string;
  topic_id: number | null;
  created_at: string;
};

export type ForumPage<T> = {
  items: T[];
  total: number;
  limit: number;
  offset: number;
};

/** Tenant routes are scoped by the active organization. */
function orgHeader() {
  return { "X-Organization": getActiveOrganizationSlug() ?? "" };
}

function orgSlug() {
  return getActiveOrganizationSlug() ?? "";
}

/**
 * Community forum (CF-78). Agents read published content through the same
 * public help center API visitors use (scoped to the active organization's
 * slug) and moderate through `/v1/tenant/forum/*` (tenant.feedback.manage;
 * conversion to a conversation also needs tenant.conversations.manage).
 */
export const forumService = {
  async categories() {
    return unwrap<ForumCategory[]>(
      await apiClient.GET(
        "/v1/public/help/{organization_slug}/forum/categories",
        { params: { path: { organization_slug: orgSlug() } } },
      ),
    );
  },

  async topics(query: {
    limit: number;
    offset: number;
    category?: string;
    q?: string;
  }) {
    return unwrap<ForumPage<ForumTopicSummary>>(
      await apiClient.GET("/v1/public/help/{organization_slug}/forum/topics", {
        params: {
          path: { organization_slug: orgSlug() },
          query: {
            limit: query.limit,
            offset: query.offset,
            ...(query.category ? { category: query.category } : {}),
            ...(query.q ? { q: query.q } : {}),
          },
        },
      }),
    );
  },

  async topic(slug: string, limit = 100, offset = 0) {
    return unwrap<ForumTopic>(
      await apiClient.GET(
        "/v1/public/help/{organization_slug}/forum/topics/{topic_slug}",
        {
          params: {
            path: { organization_slug: orgSlug(), topic_slug: slug },
            query: { limit, offset },
          },
        },
      ),
    );
  },

  async createCategory(body: { name: string; slug: string }) {
    return unwrap<ForumCategory>(
      await apiClient.POST("/v1/tenant/forum/categories", {
        params: { header: orgHeader() },
        body,
      }),
    );
  },

  async moderation(limit: number, offset: number) {
    return unwrap<ForumPage<ForumModerationItem>>(
      await apiClient.GET("/v1/tenant/forum/moderation", {
        params: { header: orgHeader(), query: { limit, offset } },
      }),
    );
  },

  async moderate(uuid: string, status: "published" | "rejected") {
    return unwrap(
      await apiClient.PATCH("/v1/tenant/forum/posts/{uuid}", {
        params: { header: orgHeader(), path: { uuid } },
        body: { status },
      }),
    );
  },

  /** Explicit, credit-metered AI triage; saves a suggestion only. */
  async assist(uuid: string) {
    return unwrap<{ flagged: boolean; reason: string }>(
      await apiClient.POST("/v1/tenant/forum/posts/{uuid}/assist", {
        params: { header: orgHeader(), path: { uuid } },
      }),
    );
  },

  async accept(topicUuid: string, replyUuid: string) {
    return unwrap<{ accepted: boolean }>(
      await apiClient.POST("/v1/tenant/forum/topics/{uuid}/accept", {
        params: { header: orgHeader(), path: { uuid: topicUuid } },
        body: { reply_uuid: replyUuid },
      }),
    );
  },

  /** New pending topic from a conversation, with agent-redacted text. */
  async fromConversation(body: ForumConversion) {
    return unwrap<Record<string, unknown>>(
      await apiClient.POST("/v1/tenant/forum/from-conversation", {
        params: { header: orgHeader() },
        body,
      }),
    );
  },

  /** Opens (or returns the already linked) conversation for a topic. */
  async toConversation(topicUuid: string, inboxUuid: string) {
    return unwrap<{ uuid?: string }>(
      await apiClient.POST("/v1/tenant/forum/topics/{uuid}/conversation", {
        params: { header: orgHeader(), path: { uuid: topicUuid } },
        body: { inbox_uuid: inboxUuid },
      }),
    );
  },
};

/** URL-safe slug suggestion from a category name (Turkish letters folded). */
export function slugify(name: string) {
  const map: Record<string, string> = {
    ç: "c",
    ğ: "g",
    ı: "i",
    ö: "o",
    ş: "s",
    ü: "u",
  };
  return name
    .toLocaleLowerCase("tr")
    .replace(/[çğıöşü]/g, (c) => map[c] ?? c)
    .normalize("NFKD")
    .replace(/[̀-ͯ]/g, "")
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 60);
}
