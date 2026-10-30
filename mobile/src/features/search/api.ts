import { useQuery } from '@tanstack/react-query';

import { api, orgHeader, unwrap } from '@/core/api/client';
import { orgKey, useOrgSlug } from '@/core/api/org';
import type { components } from '@/core/api/schema';
import { P, useCan } from '@/core/auth/permissions';
import { readJSON, writeJSON } from '@/core/storage';

/**
 * Global search (the panel's Cmd+K): `/v1/tenant/search` over conversations,
 * messages, contacts and companies (RBAC-filtered by the API), plus knowledge
 * articles from `/v1/tenant/knowledge/search` when the member may read them.
 */
export type SearchHit = components['schemas']['SearchHit'];
export type SearchSpec = 'conversations' | 'messages' | 'contacts' | 'companies';
export type ArticleHit = { uuid: string; title: string; snippet: string };

const MIN = 2;

export function useGlobalSearch(q: string, spec: SearchSpec | null) {
  const org = useOrgSlug();
  const query = q.trim();
  return useQuery({
    queryKey: orgKey(org, 'search', query, spec),
    enabled: !!org && query.length >= MIN,
    staleTime: 30_000,
    placeholderData: (prev) => prev,
    queryFn: () =>
      unwrap(
        api.GET('/v1/tenant/search', {
          params: { header: orgHeader(), query: { q: query, limit: spec ? 30 : 20, ...(spec ? { spec } : {}) } },
        }),
      ),
  });
}

/** Knowledge base hits, one per article (the API returns passages). */
export function useArticleSearch(q: string, enabled = true) {
  const org = useOrgSlug();
  const can = useCan();
  const query = q.trim();
  return useQuery({
    queryKey: orgKey(org, 'kb-search', query),
    enabled: !!org && enabled && can(P.settingsRead) && query.length >= MIN,
    staleTime: 60_000,
    placeholderData: (prev) => prev,
    queryFn: async (): Promise<ArticleHit[]> => {
      const r = await unwrap(
        api.GET('/v1/tenant/knowledge/search', { params: { header: orgHeader(), query: { q: query } } }),
      );
      const seen = new Map<string, ArticleHit>();
      for (const c of r) {
        if (!seen.has(c.article_uuid))
          seen.set(c.article_uuid, { uuid: c.article_uuid, title: c.title, snippet: c.content.slice(0, 160) });
      }
      return [...seen.values()];
    },
  });
}

const RECENT_KEY = 'search.recent';

/** The last searches on this phone (Superhuman / Linear style quick re-run). */
export function recentSearches(): string[] {
  return readJSON<string[]>(RECENT_KEY) ?? [];
}

export function rememberSearch(q: string) {
  const v = q.trim();
  if (v.length < MIN) return;
  writeJSON(RECENT_KEY, [v, ...recentSearches().filter((x) => x !== v)].slice(0, 8));
}

export function clearRecentSearches() {
  writeJSON(RECENT_KEY, []);
}
