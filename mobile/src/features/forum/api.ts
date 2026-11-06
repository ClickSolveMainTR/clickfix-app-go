import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { api, orgHeader, unwrap } from '@/core/api/client';
import { orgKey, useOrgSlug } from '@/core/api/org';
import type { components } from '@/core/api/schema';
import { P, useCan } from '@/core/auth/permissions';
import { nextOffset } from '@/ui/paged-list';

import { type ModerationItem, type Outcome, outcomeStatus, readModerationItem } from './core';

/**
 * Community forum moderation (CF-171), the endpoints the panel's forum page
 * calls (`frontend/src/features/forum/services/forum.service.ts`): the queue
 * and its decisions under `/v1/tenant/forum/*` (`tenant.feedback.manage`),
 * published topics through the public help center API, scoped by the
 * organization's slug. Nothing is requested without the permission.
 */
export type ForumCategory = components['schemas']['ForumCategory'];
export type ForumTopicSummary = components['schemas']['ForumTopicSummary'];
export type ForumTopic = components['schemas']['ForumTopic'];
export type ForumReply = components['schemas']['ForumReply'];

const PAGE = 20;
/** Replies shown on a topic; the panel reads the same first 100. */
export const REPLY_LIMIT = 100;

const keys = {
  all: (org: string) => orgKey(org, 'forum'),
  moderation: (org: string) => [...keys.all(org), 'moderation'] as const,
  categories: (org: string) => [...keys.all(org), 'categories'] as const,
  topics: (org: string, q: string, category: string) => [...keys.all(org), 'topics', q, category] as const,
  topic: (org: string, slug: string) => [...keys.all(org), 'topic', slug] as const,
};

export function useForumScope() {
  const org = useOrgSlug();
  const can = useCan();
  return { org, canManage: can(P.feedbackManage) };
}

type ModerationPage = { items: ModerationItem[]; total: number; limit: number; offset: number };

/** Every visitor topic and reply waits here until a moderator decides. */
export function useModerationQueue() {
  const { org, canManage } = useForumScope();
  return useInfiniteQuery({
    queryKey: keys.moderation(org),
    enabled: !!org && canManage,
    initialPageParam: 0,
    staleTime: 15_000,
    queryFn: async ({ pageParam }): Promise<ModerationPage> => {
      const page = await unwrap(
        api.GET('/v1/tenant/forum/moderation', {
          params: { header: orgHeader(), query: { limit: PAGE, offset: pageParam } },
        }),
      );
      const items = page.items.map(readModerationItem).filter((i): i is ModerationItem => i !== null);
      // The next offset counts what the API sent, not what could be read.
      return { items, total: page.total, limit: page.limit, offset: page.offset + (page.items.length - items.length) };
    },
    getNextPageParam: nextOffset,
  });
}

export function useForumCategories() {
  const { org, canManage } = useForumScope();
  return useQuery({
    queryKey: keys.categories(org),
    enabled: !!org && canManage,
    staleTime: 5 * 60_000,
    queryFn: () =>
      unwrap(
        api.GET('/v1/public/help/{organization_slug}/forum/categories', {
          params: { path: { organization_slug: org } },
        }),
      ),
  });
}

type TopicPage = { items: ForumTopicSummary[]; total: number; limit: number; offset: number };

/** Published topics as visitors see them, by title and category. */
export function useForumTopics(q: string, category: string) {
  const { org, canManage } = useForumScope();
  const query = q.trim();
  return useInfiniteQuery({
    queryKey: keys.topics(org, query, category),
    enabled: !!org && canManage,
    initialPageParam: 0,
    staleTime: 30_000,
    placeholderData: (prev) => prev,
    queryFn: ({ pageParam }): Promise<TopicPage> =>
      unwrap(
        api.GET('/v1/public/help/{organization_slug}/forum/topics', {
          params: {
            path: { organization_slug: org },
            query: {
              limit: PAGE,
              offset: pageParam,
              ...(category ? { category } : {}),
              ...(query ? { q: query } : {}),
            },
          },
        }),
      ),
    getNextPageParam: (last: TopicPage) => nextOffset(last),
  });
}

export function useForumTopic(slug: string) {
  const { org, canManage } = useForumScope();
  return useQuery({
    queryKey: keys.topic(org, slug),
    enabled: !!org && canManage && !!slug,
    staleTime: 15_000,
    queryFn: () =>
      unwrap(
        api.GET('/v1/public/help/{organization_slug}/forum/topics/{topic_slug}', {
          params: { path: { organization_slug: org, topic_slug: slug }, query: { limit: REPLY_LIMIT, offset: 0 } },
        }),
      ),
  });
}

/**
 * The moderator's decisions. Each write sends only its own field (`status`,
 * `reply_uuid`), never a record rebuilt from the cache. Every success
 * refreshes the whole forum: the queue and its count, the topics, the topic.
 */
export function useForumActions() {
  const qc = useQueryClient();
  const { org } = useForumScope();
  const done = () => qc.invalidateQueries({ queryKey: keys.all(org) });
  const moderate = useMutation({
    mutationFn: ({ uuid, outcome }: { uuid: string; outcome: Outcome }) =>
      unwrap(
        api.PATCH('/v1/tenant/forum/posts/{uuid}', {
          params: { header: orgHeader(), path: { uuid } },
          body: { status: outcomeStatus(outcome) },
        }),
      ),
    onSuccess: done,
  });
  /** Explicit, credit-metered AI triage; it only saves a suggestion on the post. */
  const assist = useMutation({
    mutationFn: (uuid: string) =>
      unwrap(api.POST('/v1/tenant/forum/posts/{uuid}/assist', { params: { header: orgHeader(), path: { uuid } } })),
    onSuccess: done,
  });
  const accept = useMutation({
    mutationFn: ({ topic, reply }: { topic: string; reply: string }) =>
      unwrap(
        api.POST('/v1/tenant/forum/topics/{uuid}/accept', {
          params: { header: orgHeader(), path: { uuid: topic } },
          body: { reply_uuid: reply },
        }),
      ),
    onSuccess: done,
  });
  return { moderate, assist, accept };
}
