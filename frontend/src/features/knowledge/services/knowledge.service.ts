import type { components, operations } from "@/generated/api";
import { getActiveOrganizationSlug } from "@/features/organizations";
import { apiClient, unwrap } from "@/lib/api";

export type KnowledgeArticle = components["schemas"]["KnowledgeArticle"];
export type KnowledgeArticleInput =
  components["schemas"]["KnowledgeArticleInput"];
export type KnowledgeArticleListItem =
  components["schemas"]["KnowledgeArticleListItem"];
export type KnowledgeCollection = components["schemas"]["KnowledgeCollection"];
export type KnowledgeCollectionInput =
  components["schemas"]["KnowledgeCollectionInput"];
export type KnowledgeArticleVersion =
  components["schemas"]["KnowledgeArticleVersion"];
export type KnowledgeArticleFilters = NonNullable<
  operations["knowledgeArticles"]["parameters"]["query"]
>;

type Page<T> = { items: T[]; total: number; limit: number; offset: number };

/** Tenant routes are scoped by the active organization. */
function orgHeader() {
  return { "X-Organization": getActiveOrganizationSlug() ?? "" };
}

/** Collections are few per organization: one page of 100 covers them. */
const COLLECTION_PAGE = 100;

/**
 * Knowledge base authoring (CF-73). Save and publish run silently so the
 * editor can explain a version conflict (409) itself.
 */
export const knowledgeService = {
  async articles(filters: KnowledgeArticleFilters) {
    return unwrap<Page<KnowledgeArticleListItem>>(
      await apiClient.GET("/v1/tenant/knowledge/articles", {
        params: { header: orgHeader(), query: filters },
      }),
    );
  },

  async article(uuid: string) {
    return unwrap<KnowledgeArticle>(
      await apiClient.GET("/v1/tenant/knowledge/articles/{uuid}", {
        params: { header: orgHeader(), path: { uuid } },
      }),
    );
  },

  async create(body: KnowledgeArticleInput) {
    return unwrap<KnowledgeArticle>(
      await apiClient.POST("/v1/tenant/knowledge/articles", {
        params: { header: orgHeader() },
        body,
      }),
      { silent: true },
    );
  },

  async update(uuid: string, body: KnowledgeArticleInput) {
    return unwrap<KnowledgeArticle>(
      await apiClient.PUT("/v1/tenant/knowledge/articles/{uuid}", {
        params: { header: orgHeader(), path: { uuid } },
        body,
      }),
      { silent: true },
    );
  },

  async remove(uuid: string) {
    return unwrap(
      await apiClient.DELETE("/v1/tenant/knowledge/articles/{uuid}", {
        params: { header: orgHeader(), path: { uuid } },
      }),
    );
  },

  async publish(uuid: string, version: number, publish: boolean) {
    return unwrap(
      await apiClient.POST("/v1/tenant/knowledge/articles/{uuid}/publish", {
        params: { header: orgHeader(), path: { uuid } },
        body: { version, publish },
      }),
      { silent: true },
    );
  },

  async versions(uuid: string) {
    const page = await unwrap<Page<KnowledgeArticleVersion>>(
      await apiClient.GET("/v1/tenant/knowledge/articles/{uuid}/versions", {
        params: {
          header: orgHeader(),
          path: { uuid },
          query: { limit: 100 },
        },
      }),
    );
    return page.items;
  },

  async collections() {
    const page = await unwrap<Page<KnowledgeCollection>>(
      await apiClient.GET("/v1/tenant/knowledge/collections", {
        params: { header: orgHeader(), query: { limit: COLLECTION_PAGE } },
      }),
    );
    return page.items;
  },

  async createCollection(body: KnowledgeCollectionInput) {
    return unwrap<KnowledgeCollection>(
      await apiClient.POST("/v1/tenant/knowledge/collections", {
        params: { header: orgHeader() },
        body,
      }),
      { silent: true },
    );
  },

  async updateCollection(uuid: string, body: KnowledgeCollectionInput) {
    return unwrap<KnowledgeCollection>(
      await apiClient.PUT("/v1/tenant/knowledge/collections/{uuid}", {
        params: { header: orgHeader(), path: { uuid } },
        body,
      }),
      { silent: true },
    );
  },

  async removeCollection(uuid: string) {
    return unwrap(
      await apiClient.DELETE("/v1/tenant/knowledge/collections/{uuid}", {
        params: { header: orgHeader(), path: { uuid } },
      }),
      { silent: true },
    );
  },
};

export type KnowledgeArticleAudience =
  components["schemas"]["KnowledgeArticleAudience"];

/** Who the widget offers an article to (CF-196 catalog conditions). */
export const knowledgeAudienceService = {
  async get(uuid: string) {
    return unwrap<KnowledgeArticleAudience>(
      await apiClient.GET("/v1/tenant/knowledge/articles/{uuid}/audience", {
        params: { header: orgHeader(), path: { uuid } },
      }),
    );
  },
  async set(uuid: string, body: KnowledgeArticleAudience) {
    return unwrap<KnowledgeArticleAudience>(
      await apiClient.PUT("/v1/tenant/knowledge/articles/{uuid}/audience", {
        params: { header: orgHeader(), path: { uuid } },
        body,
      }),
    );
  },
};

export const knowledgeKeys = {
  all: (org: string | undefined) => ["tenant", org, "knowledge"] as const,
  articles: (org: string | undefined, filters: KnowledgeArticleFilters) =>
    ["tenant", org, "knowledge", "articles", filters] as const,
  article: (org: string | undefined, uuid: string) =>
    ["tenant", org, "knowledge", "article", uuid] as const,
  versions: (org: string | undefined, uuid: string) =>
    ["tenant", org, "knowledge", "versions", uuid] as const,
  collections: (org: string | undefined) =>
    ["tenant", org, "knowledge", "collections"] as const,
  audience: (org: string | undefined, uuid: string) =>
    ["tenant", org, "knowledge", "audience", uuid] as const,
};
