export const forumKeys = {
  all: (org: string) => ["tenant", org, "forum"] as const,
  moderation: (org: string, offset: number) =>
    ["tenant", org, "forum", "moderation", offset] as const,
  removed: (org: string, offset: number) =>
    ["tenant", org, "forum", "removed", offset] as const,
  categories: (org: string) => ["tenant", org, "forum", "categories"] as const,
  topics: (
    org: string,
    f: { limit: number; offset: number; category?: string; q?: string },
  ) => ["tenant", org, "forum", "topics", f] as const,
  topic: (org: string, slug: string) =>
    ["tenant", org, "forum", "topic", slug] as const,
};
