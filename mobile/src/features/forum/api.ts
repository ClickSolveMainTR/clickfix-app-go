import { type InfiniteData, useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { api, orgHeader, unwrap } from '@/core/api/client';
import { isApiError } from '@/core/api/errors';
import { orgKey, useOrgSlug } from '@/core/api/org';
import type { components } from '@/core/api/schema';
import { P, useCan } from '@/core/auth/permissions';
import { errorMessage, i18n } from '@/core/i18n';
import { nextOffset } from '@/ui/paged-list';
import { toast } from '@/ui/toast';

import {
  AlreadyHandled,
  decisionProblem,
  type ModerationItem,
  type Outcome,
  outcomeStatus,
  readModerationItem,
  stillPending,
} from './core';

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

/**
 * The public forum endpoints answer `Cache-Control: public, max-age=30`. A
 * moderator must see what was just decided, so these reads skip the HTTP cache.
 */
const FRESH = { cache: 'no-store' } as const;

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
          ...FRESH,
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
          ...FRESH,
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
          ...FRESH,
          params: { path: { organization_slug: org, topic_slug: slug }, query: { limit: REPLY_LIMIT, offset: 0 } },
        }),
      ),
  });
}

const tr = (key: string, params?: Record<string, string | number>) => i18n.t(`admin:${key}`, params ?? {});

/** A refused write: the reason the app knows, else the API's own words. */
function failed(e: unknown) {
  const known = decisionProblem(e, isApiError(e) ? e.status : null);
  toast.error(known ? tr(known) : `${tr('forum.failed')} ${errorMessage(isApiError(e) ? e.code : undefined)}`);
}

/**
 * The moderator's decisions. Each write sends only its own field (`status`,
 * `reply_uuid`), never a record rebuilt from the cache. Whatever the answer,
 * the forum is read again afterwards (a refused decision usually means the
 * queue on the phone was old). Results are announced from here, so they still
 * show when the screen that started them is gone.
 */
export function useForumActions() {
  const qc = useQueryClient();
  const { org } = useForumScope();
  const refresh = () => qc.invalidateQueries({ queryKey: keys.all(org) });
  const moderate = useMutation({
    mutationFn: async ({ uuid, outcome }: { uuid: string; outcome: Outcome }) => {
      // The API takes the last writer, so a decision made from an old queue
      // could publish a post another moderator rejected: read the queue again
      // and stop when the post is no longer waiting.
      await qc.refetchQueries({ queryKey: keys.moderation(org), exact: true });
      const queue = qc.getQueryData<InfiniteData<ModerationPage>>(keys.moderation(org));
      if (!stillPending(queue?.pages, uuid)) throw new AlreadyHandled();
      return unwrap(
        api.PATCH('/v1/tenant/forum/posts/{uuid}', {
          params: { header: orgHeader(), path: { uuid } },
          body: { status: outcomeStatus(outcome) },
        }),
      );
    },
    onSuccess: (_d, v) => toast.success(tr(`forum.done_${v.outcome}`)),
    onError: failed,
    onSettled: refresh,
  });
  /** Explicit, credit-metered AI triage; it only saves a suggestion on the post. */
  const assist = useMutation({
    mutationFn: (uuid: string) =>
      unwrap(api.POST('/v1/tenant/forum/posts/{uuid}/assist', { params: { header: orgHeader(), path: { uuid } } })),
    onSuccess: (r) => toast.success(r.flagged ? tr('forum.ai_flagged', { reason: r.reason }) : tr('forum.ai_clean')),
    onError: (e) => (isApiError(e) && e.status === 429 ? toast.error(tr('forum.ai_limited')) : failed(e)),
    onSettled: refresh,
  });
  const accept = useMutation({
    mutationFn: ({ topic, reply }: { topic: ForumTopic; reply: ForumReply }) =>
      unwrap(
        api.POST('/v1/tenant/forum/topics/{uuid}/accept', {
          params: { header: orgHeader(), path: { uuid: topic.uuid } },
          body: { reply_uuid: reply.uuid },
        }),
      ),
    onSuccess: (_d, { topic, reply }) => {
      // The public API may answer from a 30-second cache; the topic on screen
      // is updated from what was just accepted instead of waiting for it.
      qc.setQueryData<ForumTopic>(keys.topic(org, topic.slug), (old) =>
        old
          ? {
              ...old,
              accepted_answer: {
                uuid: reply.uuid,
                body: reply.body,
                created_at: reply.created_at,
                updated_at: reply.updated_at,
              },
            }
          : old,
      );
      toast.success(tr('forum.accepted_done'));
    },
    onError: (e, { topic }) => {
      failed(e);
      void qc.invalidateQueries({ queryKey: keys.topic(org, topic.slug) });
    },
    onSettled: () => qc.invalidateQueries({ queryKey: [...keys.all(org), 'topics'] }),
  });
  return { moderate, assist, accept };
}
