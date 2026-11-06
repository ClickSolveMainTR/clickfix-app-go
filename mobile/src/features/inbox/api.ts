import { type InfiniteData, useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useCallback, useEffect, useMemo, useRef } from 'react';

import { attachmentFetch, api, orgHeader, unwrap } from '@/core/api/client';
import type { components } from '@/core/api/schema';
import { P, useCan } from '@/core/auth/permissions';
import { useSession } from '@/core/auth/session';
import { useChannel, useRealtime } from '@/features/realtime/client';

import {
  type ConversationPage,
  type ConversationUpdate,
  type InboxFilters,
  inboxKeys,
  type Message,
  type MessagePage,
  type OptimisticMessage,
  upsertMessage,
} from './cache';
import { listQuery } from './filters';
import { clientId } from './helpers';
import { itemsFor, type OutboxAttachment } from './outbox-core';
import { queueMessage, toOptimistic, useOutbox } from './outbox';

/**
 * Inbox data, ported from the panel's `features/inbox/hooks/use-inbox.ts`:
 * the same endpoints, query keys per organization, optimistic sends keyed by
 * `client_id` (through the offline send queue, `outbox.ts`), and the
 * `inbox:{org_uuid}` live channel.
 */

export * from './cache';

export type InboxAgent = components['schemas']['InboxAgent'];
export type ConversationSLA = components['schemas']['ConversationSLA'];
export type Contact = components['schemas']['Contact'];
export type Attachment = components['schemas']['Attachment'];
export type Canned = components['schemas']['Canned'];
export type CannedInput = components['schemas']['CannedInput'];

/** Realtime event on `inbox:{org_uuid}` (backend model.LiveEvent). */
type InboxLiveEvent = {
  type: 'conversation.created' | 'conversation.updated' | 'message.created' | 'message.updated';
  conversation_uuid: string;
  message?: Message;
};

const PAGE_SIZE = 30;

/**
 * While the live socket is down (switched off, refused, reconnecting) open
 * screens poll instead, so new customer messages still show up (CF-79).
 */
function usePollWhenNotLive(ms: number): number | false {
  const live = useRealtime((s) => s.state === 'connected');
  return live ? false : ms;
}

/** The active organization and what the member may do in its inbox. */
export function useInboxScope() {
  const org = useSession((s) => s.organization);
  const can = useCan();
  return {
    org: org?.slug ?? '',
    orgUuid: org?.uuid ?? '',
    canRead: can(P.conversationsRead),
    canReply: can(P.conversationsReply),
    canNote: can(P.conversationsNote, P.conversationsReply),
    canManage: can(P.conversationsManage),
    canAssign: can(P.conversationsAssign, P.conversationsManage),
    canContacts: can(P.contactsRead),
    canUpload: can(P.attachmentsWrite),
    canCanned: can(P.macrosRead),
    canCannedWrite: can(P.macrosWrite),
  };
}

export function useConversationList(filters: InboxFilters) {
  const { org, canRead } = useInboxScope();
  const poll = usePollWhenNotLive(30_000);
  return useInfiniteQuery({
    refetchInterval: poll,
    queryKey: inboxKeys.list(org, filters),
    enabled: !!org && canRead,
    initialPageParam: 0,
    queryFn: ({ pageParam }) =>
      unwrap(
        api.GET('/v1/tenant/conversations', {
          params: {
            header: orgHeader(),
            query: { ...listQuery(filters), limit: PAGE_SIZE, offset: pageParam },
          },
        }),
      ),
    getNextPageParam: (last: ConversationPage) =>
      last.offset + last.items.length < last.total ? last.offset + last.items.length : undefined,
    placeholderData: (prev) => prev,
    staleTime: 15_000,
  });
}

export function useInboxCounts() {
  const { org, canRead } = useInboxScope();
  return useQuery({
    queryKey: inboxKeys.counts(org),
    enabled: !!org && canRead,
    staleTime: 15_000,
    queryFn: () => unwrap(api.GET('/v1/tenant/conversations/counts', { params: { header: orgHeader() } })),
  });
}

export function useConversation(uuid: string) {
  const qc = useQueryClient();
  const { org } = useInboxScope();
  const poll = usePollWhenNotLive(30_000);
  return useQuery({
    refetchInterval: poll,
    queryKey: inboxKeys.detail(org, uuid),
    placeholderData: () => {
      const row = qc
        .getQueriesData<InfiniteData<ConversationPage>>({ queryKey: inboxKeys.lists(org) })
        .flatMap(([, data]) => data?.pages.flatMap((page) => page.items) ?? [])
        .find((c) => c.uuid === uuid);
      return row ? { ...row, tags: [], participants: [] } : undefined;
    },
    enabled: !!org && !!uuid,
    staleTime: 10_000,
    queryFn: () =>
      unwrap(api.GET('/v1/tenant/conversations/{uuid}', { params: { header: orgHeader(), path: { uuid } } })),
  });
}

export function useMessages(uuid: string) {
  const { org } = useInboxScope();
  const poll = usePollWhenNotLive(10_000);
  const query = useInfiniteQuery({
    refetchInterval: poll,
    queryKey: inboxKeys.messages(org, uuid),
    enabled: !!org && !!uuid,
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam }) =>
      unwrap(
        api.GET('/v1/tenant/conversations/{uuid}/messages', {
          params: {
            header: orgHeader(),
            path: { uuid },
            query: { limit: 50, ...(pageParam ? { before: pageParam } : {}) },
          },
        }),
      ),
    getNextPageParam: (last: MessagePage) => (last.has_more && last.before ? last.before : undefined),
    staleTime: 30_000,
  });
  const outbox = useOutbox((s) => s.items);
  // Pages arrive newest first and each is oldest first; the inverted list wants newest first.
  // Queued sends the cache lost (a relaunch before they went out) come back from the queue.
  const items = useMemo(() => {
    const cached = (query.data?.pages ?? []).flatMap((p) => [...p.items].reverse()) as OptimisticMessage[];
    const known = new Set(cached.map((m) => m.client_id).filter(Boolean));
    const waiting = itemsFor(outbox, org, uuid)
      .filter((i) => !known.has(i.client_id))
      .map((i) => toOptimistic(i, !i.failed))
      .reverse();
    return waiting.length ? [...waiting, ...cached] : cached;
  }, [query.data, outbox, org, uuid]);
  return { ...query, items };
}

/**
 * Sends a reply or note through the send queue: the bubble shows at once,
 * waits for the network when there is none, and fails only on a refusal.
 */
export function useSendMessage(uuid: string) {
  const { org } = useInboxScope();
  const me = useSession((s) => s.me);
  return useCallback(
    (input: { content: string; private: boolean; attachments?: OutboxAttachment[]; mentions?: string[] }) => {
      const user = me?.user;
      queueMessage({
        org,
        conversation: uuid,
        client_id: clientId(),
        content: input.content,
        private: input.private,
        attachments: input.attachments ?? [],
        mentions: input.mentions,
        sender: { uuid: user?.uuid ?? null, name: [user?.name, user?.surname].filter(Boolean).join(' ') },
        created_at: new Date().toISOString(),
      });
    },
    [me, org, uuid],
  );
}

export function useRetryMessage(uuid: string) {
  return useMutation({
    mutationFn: (message: string) =>
      unwrap(
        api.POST('/v1/tenant/conversations/{uuid}/messages/{message}/retry', {
          params: { header: orgHeader(), path: { uuid, message } },
        }),
      ),
  });
}

export function useUpdateConversation(uuid: string) {
  const { org } = useInboxScope();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: ConversationUpdate) =>
      unwrap(
        api.PATCH('/v1/tenant/conversations/{uuid}', {
          params: { header: orgHeader(), path: { uuid } },
          body,
        }),
      ),
    onSuccess: (detail) => qc.setQueryData(inboxKeys.detail(org, uuid), detail),
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: inboxKeys.sla(org, uuid) });
      void qc.invalidateQueries({ queryKey: inboxKeys.lists(org) });
      void qc.invalidateQueries({ queryKey: inboxKeys.dashboardSla(org) });
      void qc.invalidateQueries({ queryKey: inboxKeys.counts(org) });
      void qc.invalidateQueries({ queryKey: inboxKeys.messages(org, uuid) });
    },
  });
}

/** Marks the conversation read for the member and clears its unread badge in cached lists. */
export function useMarkRead(uuid: string) {
  const { org } = useInboxScope();
  const qc = useQueryClient();
  return useCallback(() => {
    if (!org || !uuid) return;
    void unwrap(api.POST('/v1/tenant/conversations/{uuid}/read', { params: { header: orgHeader(), path: { uuid } } }))
      .then(() => {
        void qc.invalidateQueries({ queryKey: inboxKeys.counts(org) });
        qc.setQueriesData<InfiniteData<ConversationPage>>({ queryKey: inboxKeys.lists(org) }, (data) =>
          data
            ? {
                ...data,
                pages: data.pages.map((p) => ({
                  ...p,
                  items: p.items.map((c) => (c.uuid === uuid ? { ...c, unread_count: 0 } : c)),
                })),
              }
            : data,
        );
      })
      .catch(() => undefined);
  }, [org, qc, uuid]);
}

/**
 * Subscribes to the organization's inbox channel: messages land in the open
 * conversation at once; lists and badges refetch (debounced). `onInbound`
 * fires for a customer message in `activeUuid` (the open screen marks it read).
 */
export function useInboxLive(activeUuid?: string, onInbound?: () => void) {
  const { org, orgUuid, canRead } = useInboxScope();
  const qc = useQueryClient();
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(
    () => () => {
      if (timer.current) clearTimeout(timer.current);
    },
    [],
  );

  useChannel(orgUuid && canRead ? `inbox:${orgUuid}` : null, (raw) => {
    const ev = raw as InboxLiveEvent | null;
    if (!ev?.type || !ev.conversation_uuid) return;
    if (ev.message) upsertMessage(qc, inboxKeys.messages(org, ev.conversation_uuid), ev.message);
    if (ev.type === 'conversation.updated' || ev.type === 'conversation.created')
      void qc.invalidateQueries({ queryKey: inboxKeys.detail(org, ev.conversation_uuid) });
    if (ev.conversation_uuid === activeUuid && ev.message?.direction === 'in') onInbound?.();
    if (timer.current) clearTimeout(timer.current);
    timer.current = setTimeout(() => {
      void qc.invalidateQueries({ queryKey: inboxKeys.lists(org) });
      void qc.invalidateQueries({ queryKey: inboxKeys.dashboardSla(org) });
      void qc.invalidateQueries({ queryKey: inboxKeys.counts(org) });
    }, 250);
  });
}

/** Members a conversation can be assigned to (`/v1/tenant/conversations/agents`). */
export function useAgents(enabled = true) {
  const { org } = useInboxScope();
  return useQuery({
    queryKey: inboxKeys.agents(org),
    enabled: !!org && enabled,
    staleTime: 5 * 60_000,
    queryFn: () => unwrap(api.GET('/v1/tenant/conversations/agents', { params: { header: orgHeader() } })),
  });
}

/** SLA policy and per-metric clocks of a conversation. */
export function useConversationSla(uuid: string) {
  const { org } = useInboxScope();
  return useQuery({
    queryKey: inboxKeys.sla(org, uuid),
    enabled: !!org && !!uuid,
    staleTime: 30_000,
    queryFn: () =>
      unwrap(api.GET('/v1/tenant/conversations/{uuid}/sla', { params: { header: orgHeader(), path: { uuid } } })),
  });
}

/** The contact card (needs `tenant.contacts.read`). */
export function useContact(uuid: string | undefined) {
  const { org, canContacts } = useInboxScope();
  return useQuery({
    queryKey: inboxKeys.contact(org, uuid ?? ''),
    enabled: !!org && !!uuid && canContacts,
    staleTime: 60_000,
    queryFn: () =>
      unwrap(
        api.GET('/v1/tenant/contacts/{uuid}', {
          params: { header: orgHeader(), path: { uuid: uuid ?? '' } },
        }),
      ),
  });
}

/** A picked file: a local `file://` URI with its name and media type. */
export type LocalFile = { uri: string; name: string; type: string };

/**
 * Uploads one file as an unattached upload of the caller; the message send
 * attaches it by uuid. React Native's FormData takes `{ uri, name, type }`.
 */
export function uploadAttachment(file: LocalFile, onProgress?: (fraction: number) => void) {
  const form = new FormData();
  form.append('file', { uri: file.uri, name: file.name, type: file.type } as unknown as Blob);
  return unwrap(
    api.POST('/v1/tenant/attachments', {
      params: { header: orgHeader() },
      // openapi-fetch leaves FormData alone, so fetch sets the multipart boundary.
      body: form as unknown as { file: string },
      bodySerializer: (b) => b as unknown as FormData,
      ...(onProgress ? { fetch: (request: Request) => attachmentFetch(request, { body: form, onProgress }) } : {}),
    }),
  );
}

/** A short-lived public URL for opening a file outside the app (browser, share sheet). */
export function signedAttachmentUrl(uuid: string) {
  return unwrap(
    api.POST('/v1/tenant/attachments/{uuid}/signed-url', {
      params: { header: orgHeader(), path: { uuid } },
      body: { variant: 'original' },
    }),
  );
}

/** Canned responses the member may use (organization-wide and personal, most used first; CF-48). */
export function useCannedResponses(q: string, enabled: boolean) {
  const { org, canCanned } = useInboxScope();
  const query = q.trim();
  return useQuery({
    queryKey: [...inboxKeys.all(org), 'canned', query] as const,
    enabled: !!org && canCanned && enabled,
    staleTime: 60_000,
    placeholderData: (prev) => prev,
    queryFn: () =>
      unwrap(
        api.GET('/v1/tenant/canned-responses', {
          params: { header: orgHeader(), query: query ? { q: query } : {} },
        }),
      ),
  });
}

/**
 * Create / edit / delete a canned response (CF-165; the panel's
 * `useCannedMutations`). Every cached search of the list is refetched, so the
 * composer's "/" picker shows the change at once.
 */
export function useCannedMutations() {
  const { org } = useInboxScope();
  const qc = useQueryClient();
  const done = () => void qc.invalidateQueries({ queryKey: [...inboxKeys.all(org), 'canned'] });
  return {
    create: useMutation({
      mutationFn: (body: CannedInput) =>
        unwrap(api.POST('/v1/tenant/canned-responses', { params: { header: orgHeader() }, body })),
      onSuccess: done,
    }),
    update: useMutation({
      mutationFn: (v: { uuid: string; body: CannedInput }) =>
        unwrap(
          api.PATCH('/v1/tenant/canned-responses/{uuid}', {
            params: { header: orgHeader(), path: { uuid: v.uuid } },
            body: v.body,
          }),
        ),
      onSuccess: done,
    }),
    remove: useMutation({
      mutationFn: (uuid: string) =>
        unwrap(api.DELETE('/v1/tenant/canned-responses/{uuid}', { params: { header: orgHeader(), path: { uuid } } })),
      onSuccess: done,
    }),
  };
}

/** A canned response with the conversation's variables filled in (also counts the use). */
export function renderCanned(uuid: string, conversation: string) {
  return unwrap(
    api.POST('/v1/tenant/canned-responses/{uuid}/render', {
      params: { header: orgHeader(), path: { uuid } },
      body: { conversation },
    }),
  );
}

/**
 * Status / assignee changes from the list (swipe and long-press actions,
 * CF-81) for any conversation, without opening it. Lists and badges refetch
 * afterwards; the open conversation's cache is updated when it is loaded.
 */
export function useQuickUpdate() {
  const { org } = useInboxScope();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ uuid, body }: { uuid: string; body: ConversationUpdate }) =>
      unwrap(
        api.PATCH('/v1/tenant/conversations/{uuid}', {
          params: { header: orgHeader(), path: { uuid } },
          body,
        }),
      ),
    onSuccess: (detail) => qc.setQueryData(inboxKeys.detail(org, detail.uuid), detail),
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: inboxKeys.lists(org) });
      void qc.invalidateQueries({ queryKey: inboxKeys.dashboardSla(org) });
      void qc.invalidateQueries({ queryKey: inboxKeys.counts(org) });
    },
  });
}

export type SavedView = components['schemas']['ConversationView'];
export type Tag = components['schemas']['Tag'];
export type BulkAction = components['schemas']['ConversationBulkInput']['action'];

/** The member's saved views (personal, team and organization) and their open counts. */
export function useSavedViews() {
  const { org, canRead } = useInboxScope();
  const views = useQuery({
    queryKey: [...inboxKeys.all(org), 'views'] as const,
    enabled: !!org && canRead,
    staleTime: 5 * 60_000,
    queryFn: () => unwrap(api.GET('/v1/tenant/conversation-views', { params: { header: orgHeader() } })),
  });
  const counts = useQuery({
    queryKey: [...inboxKeys.counts(org), 'views'] as const,
    enabled: !!org && canRead && !!views.data?.items.length,
    staleTime: 30_000,
    queryFn: () => unwrap(api.GET('/v1/tenant/conversation-views/counts', { params: { header: orgHeader() } })),
  });
  return { views: views.data?.items ?? [], counts: (counts.data?.counts ?? {}) as Record<string, number> };
}

export function useTags(enabled = true) {
  const { org } = useInboxScope();
  return useQuery({
    queryKey: [...inboxKeys.all(org), 'tags'] as const,
    enabled: !!org && enabled,
    staleTime: 5 * 60_000,
    queryFn: () => unwrap(api.GET('/v1/tenant/tags', { params: { header: orgHeader() } })),
  });
}

export function useTeams(enabled = true) {
  const { org } = useInboxScope();
  const can = useCan();
  return useQuery({
    queryKey: [...inboxKeys.all(org), 'teams'] as const,
    enabled: !!org && enabled && can(P.teamsRead),
    staleTime: 5 * 60_000,
    queryFn: () => unwrap(api.GET('/v1/tenant/teams', { params: { header: orgHeader(), query: { limit: 100 } } })),
  });
}

/** Channel types the organization has connected (filter choices). */
export function useChannelTypes(enabled = true) {
  const { org } = useInboxScope();
  const can = useCan();
  return useQuery({
    queryKey: [...inboxKeys.all(org), 'channel-types'] as const,
    enabled: !!org && enabled && can(P.channelsRead),
    staleTime: 10 * 60_000,
    queryFn: async () => {
      const list = await unwrap(api.GET('/v1/tenant/channels', { params: { header: orgHeader() } }));
      return [...new Set(list.items.map((c) => c.type))].sort();
    },
  });
}

/**
 * Bulk actions on selected conversations (`POST /conversations/bulk`, CF-79):
 * assign, status, priority, snooze, tag, untag. The answer names an operation
 * that `useUndoBulk` reverses.
 */
export function useBulk() {
  const { org } = useInboxScope();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: components['schemas']['ConversationBulkInput']) =>
      unwrap(api.POST('/v1/tenant/conversations/bulk', { params: { header: orgHeader() }, body })),
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: inboxKeys.lists(org) });
      void qc.invalidateQueries({ queryKey: inboxKeys.dashboardSla(org) });
      void qc.invalidateQueries({ queryKey: inboxKeys.counts(org) });
    },
  });
}

export function useUndoBulk() {
  const { org } = useInboxScope();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (operation: string) =>
      unwrap(
        api.POST('/v1/tenant/conversations/bulk/undo/{uuid}', {
          params: { header: orgHeader(), path: { uuid: operation } },
        }),
      ),
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: inboxKeys.lists(org) });
      void qc.invalidateQueries({ queryKey: inboxKeys.dashboardSla(org) });
      void qc.invalidateQueries({ queryKey: inboxKeys.counts(org) });
    },
  });
}
