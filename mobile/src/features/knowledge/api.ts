import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { api, orgHeader, unwrap, unwrapVoid } from '@/core/api/client';
import { orgKey, useOrgSlug } from '@/core/api/org';
import type { components } from '@/core/api/schema';
import { P, useCan } from '@/core/auth/permissions';

/**
 * Knowledge base (`/v1/tenant/knowledge/*`, behind `tenant.settings.read` /
 * `.write` like the panel's Settings → Knowledge): collections, articles,
 * quick edits of a draft and publishing.
 */
export type ArticleListItem = components['schemas']['KnowledgeArticleListItem'];
export type Article = components['schemas']['KnowledgeArticle'];
export type Collection = components['schemas']['KnowledgeCollection'];
export type ArticleInput = components['schemas']['KnowledgeArticleInput'];

const PAGE = 30;
export const kbKeys = {
  all: (org: string) => orgKey(org, 'kb'),
  collections: (org: string) => [...kbKeys.all(org), 'collections'] as const,
  articles: (org: string, collection: string | null, q: string) =>
    [...kbKeys.all(org), 'articles', collection, q] as const,
  article: (org: string, uuid: string) => [...kbKeys.all(org), 'article', uuid] as const,
};

export function useKnowledgeScope() {
  const can = useCan();
  return { canRead: can(P.settingsRead), canWrite: can(P.settingsWrite) };
}

export function useCollections() {
  const org = useOrgSlug();
  const { canRead } = useKnowledgeScope();
  return useQuery({
    queryKey: kbKeys.collections(org),
    enabled: !!org && canRead,
    staleTime: 5 * 60_000,
    queryFn: () =>
      unwrap(api.GET('/v1/tenant/knowledge/collections', { params: { header: orgHeader(), query: { limit: 100 } } })),
  });
}

export function useArticles(collection: string | null, q = '') {
  const org = useOrgSlug();
  const { canRead } = useKnowledgeScope();
  return useInfiniteQuery({
    queryKey: kbKeys.articles(org, collection, q.trim()),
    enabled: !!org && canRead,
    initialPageParam: 0,
    staleTime: 60_000,
    queryFn: ({ pageParam }) =>
      unwrap(
        api.GET('/v1/tenant/knowledge/articles', {
          params: {
            header: orgHeader(),
            query: {
              limit: PAGE,
              offset: pageParam,
              ...(collection ? { collection_uuid: collection } : {}),
              ...(q.trim() ? { q: q.trim() } : {}),
            },
          },
        }),
      ),
    getNextPageParam: (last) =>
      last.offset + last.items.length < last.total ? last.offset + last.items.length : undefined,
  });
}

export function useArticle(uuid: string) {
  const org = useOrgSlug();
  const { canRead } = useKnowledgeScope();
  return useQuery({
    queryKey: kbKeys.article(org, uuid),
    enabled: !!org && !!uuid && canRead,
    staleTime: 60_000,
    queryFn: () =>
      unwrap(api.GET('/v1/tenant/knowledge/articles/{uuid}', { params: { header: orgHeader(), path: { uuid } } })),
  });
}

/** Saves a quick edit (title / body); the API bumps the version, publishing is a separate step. */
export function useSaveArticle(uuid: string) {
  const org = useOrgSlug();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: ArticleInput) =>
      unwrap(
        api.PUT('/v1/tenant/knowledge/articles/{uuid}', { params: { header: orgHeader(), path: { uuid } }, body }),
      ),
    onSuccess: (a) => qc.setQueryData(kbKeys.article(org, uuid), a),
    onSettled: () => void qc.invalidateQueries({ queryKey: kbKeys.all(org) }),
  });
}

export function usePublishArticle(uuid: string) {
  const org = useOrgSlug();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (version: number) =>
      unwrapVoid(
        api.POST('/v1/tenant/knowledge/articles/{uuid}/publish', {
          params: { header: orgHeader(), path: { uuid } },
          body: { version, publish: true },
        }),
      ),
    onSettled: () => void qc.invalidateQueries({ queryKey: kbKeys.all(org) }),
  });
}
