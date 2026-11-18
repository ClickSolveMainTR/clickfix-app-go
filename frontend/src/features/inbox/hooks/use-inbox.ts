"use client";

import { useCallback, useEffect, useMemo, useRef } from "react";
import {
  useInfiniteQuery,
  useQuery,
  useQueryClient,
  type InfiniteData,
  type QueryClient,
} from "@tanstack/react-query";

import { Permission } from "@/config/permissions";
import { useActiveOrganization } from "@/features/organizations";
import { useChannel } from "@/hooks/use-realtime";
import { RealtimeChannels } from "@/lib/realtime/channels";
import { useAppMutation } from "@/lib/query/mutation";
import { useAuth } from "@/providers/auth-provider";

import { inboxService } from "../services/inbox.service";
import type {
  ConversationDetail,
  ConversationPage,
  ConversationUpdate,
  ConversationViewInput,
  InboxFilters,
  InboxLiveEvent,
  Message,
  MessageCreate,
  MessagePage,
  OptimisticMessage,
} from "../types";

const PAGE_SIZE = 40;

export const inboxKeys = {
  all: (org: string) => ["inbox", org] as const,
  lists: (org: string) => [...inboxKeys.all(org), "list"] as const,
  list: (org: string, f: InboxFilters) => [...inboxKeys.lists(org), f] as const,
  counts: (org: string) => [...inboxKeys.all(org), "counts"] as const,
  agents: (org: string) => [...inboxKeys.all(org), "agents"] as const,
  aiAvailability: (org: string) =>
    [...inboxKeys.all(org), "ai-availability"] as const,
  tags: (org: string) => [...inboxKeys.all(org), "tags"] as const,
  customStatuses: (org: string) =>
    [...inboxKeys.all(org), "custom-statuses"] as const,
  detail: (org: string, uuid: string) =>
    [...inboxKeys.all(org), "detail", uuid] as const,
  messages: (org: string, uuid: string) =>
    [...inboxKeys.all(org), "messages", uuid] as const,
  /** Autonomous agent jobs of a conversation (composer draft band). */
  aiJobs: (org: string, uuid: string) =>
    [...inboxKeys.all(org), "ai-jobs", uuid] as const,
  history: (org: string, contact: string) =>
    [...inboxKeys.all(org), "history", contact] as const,
};

/** The active organization and what the caller may do in its inbox. */
export function useInboxScope() {
  const { active } = useActiveOrganization();
  const perms = useMemo(
    () => new Set<string>(active?.permissions ?? []),
    [active?.permissions],
  );
  return {
    org: active?.slug ?? "",
    orgUuid: active?.uuid ?? "",
    canRead: perms.has(Permission.TenantConversationsRead),
    /** Sees every conversation; otherwise only its teams' (CF-44). */
    canReadAll: perms.has(Permission.TenantConversationsReadAll),
    canReply: perms.has(Permission.TenantConversationsReply),
    canManage: perms.has(Permission.TenantConversationsManage),
    canNote:
      perms.has(Permission.TenantConversationsNote) ||
      perms.has(Permission.TenantConversationsReply),
    canAssign:
      perms.has(Permission.TenantConversationsAssign) ||
      perms.has(Permission.TenantConversationsManage),
  };
}

export function useConversationList(filters: InboxFilters) {
  const { org, canRead } = useInboxScope();
  const query = useInfiniteQuery({
    queryKey: inboxKeys.list(org, filters),
    queryFn: ({ pageParam }) =>
      inboxService.list({ ...filters, limit: PAGE_SIZE, offset: pageParam }),
    initialPageParam: 0,
    getNextPageParam: (last: ConversationPage) =>
      last.offset + last.items.length < last.total
        ? last.offset + last.items.length
        : undefined,
    enabled: Boolean(org) && canRead,
    placeholderData: (prev) => prev,
    staleTime: 15_000,
  });
  const items = useMemo(() => {
    const seen = new Set<string>();
    return (query.data?.pages ?? [])
      .flatMap((p) => p.items)
      .filter((c) => (seen.has(c.uuid) ? false : (seen.add(c.uuid), true)));
  }, [query.data]);
  const total = query.data?.pages[0]?.total ?? 0;
  return { ...query, items, total };
}

export function useInboxCounts() {
  const { org, canRead } = useInboxScope();
  return useQuery({
    queryKey: inboxKeys.counts(org),
    queryFn: () => inboxService.counts(),
    enabled: Boolean(org) && canRead,
    staleTime: 15_000,
  });
}

export function useInboxAgents() {
  const { org, canRead } = useInboxScope();
  return useQuery({
    queryKey: inboxKeys.agents(org),
    queryFn: () => inboxService.agents(),
    enabled: Boolean(org) && canRead,
    staleTime: 5 * 60_000,
  });
}

export function useInboxTags() {
  const { org, canRead } = useInboxScope();
  return useQuery({
    queryKey: inboxKeys.tags(org),
    queryFn: () => inboxService.tags(),
    enabled: Boolean(org) && canRead,
    staleTime: 60_000,
  });
}

/** Custom statuses for the status picker (CF-249). */
export function useInboxCustomStatuses() {
  const { org, canRead } = useInboxScope();
  return useQuery({
    queryKey: inboxKeys.customStatuses(org),
    queryFn: () => inboxService.customStatuses(),
    enabled: Boolean(org) && canRead,
    staleTime: 60_000,
  });
}

export function useConversation(uuid: string | null) {
  const { org } = useInboxScope();
  return useQuery({
    queryKey: inboxKeys.detail(org, uuid ?? ""),
    queryFn: () => inboxService.get(uuid as string),
    enabled: Boolean(org && uuid),
    staleTime: 10_000,
  });
}

export function useContactHistory(contact: string | undefined) {
  const { org } = useInboxScope();
  return useQuery({
    queryKey: inboxKeys.history(org, contact ?? ""),
    queryFn: () =>
      inboxService.list({
        view: "all",
        status: "all",
        q: "",
        unread: false,
        sort: "-last_message_at",
        contact,
        limit: 6,
        offset: 0,
      }),
    enabled: Boolean(org && contact),
    staleTime: 30_000,
  });
}

type MessagesData = InfiniteData<MessagePage, string | undefined>;

export function useMessages(uuid: string | null) {
  const { org } = useInboxScope();
  const query = useInfiniteQuery({
    queryKey: inboxKeys.messages(org, uuid ?? ""),
    queryFn: ({ pageParam }) =>
      inboxService.messages(uuid as string, pageParam),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last: MessagePage) =>
      last.has_more && last.before ? last.before : undefined,
    enabled: Boolean(org && uuid),
    staleTime: 30_000,
  });
  // Pages arrive newest first; render oldest first.
  const items = useMemo(
    () =>
      [...(query.data?.pages ?? [])]
        .reverse()
        .flatMap((p) => p.items) as OptimisticMessage[],
    [query.data],
  );
  return { ...query, items };
}

/** Inserts or replaces a message in the cached pages (by uuid or client_id). */
export function upsertMessage(
  qc: QueryClient,
  key: readonly unknown[],
  msg: OptimisticMessage,
) {
  qc.setQueryData<MessagesData>(key, (data) => {
    if (!data) return data;
    const same = (m: Message) =>
      m.uuid === msg.uuid ||
      (msg.client_id != null && m.client_id === msg.client_id);
    let found = false;
    const pages = data.pages.map((p) => {
      if (!p.items.some(same)) return p;
      found = true;
      return {
        ...p,
        items: p.items.map((m) =>
          same(m) ? { ...m, ...msg, optimistic: undefined } : m,
        ),
      };
    });
    if (found) return { ...data, pages };
    const [newest, ...rest] = pages;
    if (!newest) return data;
    return {
      ...data,
      pages: [{ ...newest, items: [...newest.items, msg] }, ...rest],
    };
  });
}

export function useSendMessage(uuid: string) {
  const { org } = useInboxScope();
  const { user } = useAuth();
  const qc = useQueryClient();
  const key = inboxKeys.messages(org, uuid);
  return useAppMutation({
    mutationFn: (body: MessageCreate & { client_id: string }) =>
      inboxService.post(uuid, body),
    onMutate: async (body) => {
      await qc.cancelQueries({ queryKey: key });
      const now = new Date().toISOString();
      upsertMessage(qc, key, {
        uuid: body.client_id,
        conversation_uuid: uuid,
        direction: body.private ? "internal" : "out",
        sender: {
          type: "agent",
          uuid: user?.uuid ?? null,
          name: [user?.name, user?.surname].filter(Boolean).join(" "),
        },
        kind: body.private ? "note" : "text",
        content: body.content ?? "",
        attributes: {},
        private: Boolean(body.private),
        status: "pending",
        error: "",
        client_id: body.client_id,
        reply_to: body.reply_to ?? null,
        mentions: [],
        attachments: [],
        sent_at: null,
        delivered_at: null,
        read_at: null,
        created_at: now,
        optimistic: true,
      });
    },
    onSuccess: (msg) => upsertMessage(qc, key, msg),
    onError: (_err, body) => {
      upsertMessage(qc, key, {
        ...(qc
          .getQueryData<MessagesData>(key)
          ?.pages.flatMap((p) => p.items)
          .find((m) => m.client_id === body.client_id) as Message),
        status: "failed",
        error: "not_sent",
      });
    },
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: inboxKeys.lists(org) });
      // First response, waiting time and message count (and the SLA panel
      // keyed on them) live on the detail.
      void qc.invalidateQueries({ queryKey: inboxKeys.detail(org, uuid) });
    },
  });
}

export function useRetryMessage(uuid: string) {
  return useAppMutation({
    mutationFn: (message: string) => inboxService.retry(uuid, message),
  });
}

export function useUpdateConversation(uuid: string) {
  const { org } = useInboxScope();
  const qc = useQueryClient();
  const key = inboxKeys.detail(org, uuid);
  return useAppMutation({
    mutationFn: (body: ConversationUpdate) => inboxService.update(uuid, body),
    onMutate: async (body) => {
      await qc.cancelQueries({ queryKey: key });
      const prev = qc.getQueryData<ConversationDetail>(key);
      if (prev) {
        qc.setQueryData<ConversationDetail>(key, {
          ...prev,
          ...(body.status ? { status: body.status } : {}),
          ...(body.priority ? { priority: body.priority } : {}),
          ...(body.custom_status === null ? { custom_status: null } : {}),
        });
      }
      return { prev };
    },
    onError: (_e, _b, ctx) => {
      if (ctx?.prev) qc.setQueryData(key, ctx.prev);
    },
    onSuccess: (data) => qc.setQueryData(key, data),
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: inboxKeys.lists(org) });
      void qc.invalidateQueries({ queryKey: inboxKeys.counts(org) });
      void qc.invalidateQueries({ queryKey: inboxKeys.messages(org, uuid) });
    },
  });
}

export function useConversationTag(uuid: string) {
  const { org } = useInboxScope();
  const qc = useQueryClient();
  const settle = () => {
    void qc.invalidateQueries({ queryKey: inboxKeys.detail(org, uuid) });
    void qc.invalidateQueries({ queryKey: inboxKeys.tags(org) });
    void qc.invalidateQueries({ queryKey: inboxKeys.lists(org) });
  };
  const add = useAppMutation({
    mutationFn: (tag: { uuid?: string; name?: string }) =>
      inboxService.addTag(uuid, tag),
    onSettled: settle,
  });
  const remove = useAppMutation({
    mutationFn: (tag: string) => inboxService.removeTag(uuid, tag),
    onSettled: settle,
  });
  return { add, remove };
}

export function useMarkRead(uuid: string | null) {
  const { org } = useInboxScope();
  const qc = useQueryClient();
  return useCallback(() => {
    if (!uuid || document.visibilityState !== "visible") return;
    void inboxService.markRead(uuid).then(() => {
      void qc.invalidateQueries({ queryKey: inboxKeys.counts(org) });
      qc.setQueriesData<InfiniteData<ConversationPage>>(
        { queryKey: inboxKeys.lists(org) },
        (data) =>
          data && {
            ...data,
            pages: data.pages.map((p) => ({
              ...p,
              items: p.items.map((c) =>
                c.uuid === uuid ? { ...c, unread_count: 0 } : c,
              ),
            })),
          },
      );
    });
  }, [org, qc, uuid]);
}

/**
 * Marks conversations read or unread by hand (CF-247): the list row and
 * the badges change at once, then refetch.
 */
export function useReadState() {
  const { org } = useInboxScope();
  const qc = useQueryClient();
  const patch = useCallback(
    (uuid: string, unread: boolean) =>
      qc.setQueriesData<InfiniteData<ConversationPage>>(
        { queryKey: inboxKeys.lists(org) },
        (data) =>
          data && {
            ...data,
            pages: data.pages.map((p) => ({
              ...p,
              items: p.items.map((c) =>
                c.uuid === uuid
                  ? {
                      ...c,
                      unread_count: unread ? Math.max(c.unread_count, 1) : 0,
                    }
                  : c,
              ),
            })),
          },
      ),
    [org, qc],
  );
  const settle = useCallback(
    (uuid: string) => {
      void qc.invalidateQueries({ queryKey: inboxKeys.lists(org) });
      void qc.invalidateQueries({ queryKey: inboxKeys.counts(org) });
      void qc.invalidateQueries({ queryKey: inboxKeys.detail(org, uuid) });
    },
    [org, qc],
  );
  const markUnread = useAppMutation({
    mutationFn: (uuid: string) => inboxService.markUnread(uuid),
    onMutate: (uuid) => patch(uuid, true),
    onSettled: (_d, _e, uuid) => settle(uuid),
  });
  const markRead = useAppMutation({
    mutationFn: (uuid: string) => inboxService.markRead(uuid),
    onMutate: (uuid) => patch(uuid, false),
    onSettled: (_d, _e, uuid) => settle(uuid),
  });
  return { markRead, markUnread };
}

/**
 * Subscribes to the inbox channel: messages land in the open conversation
 * instantly; lists and badges refetch (debounced). Members with read_all
 * listen on the organization channel, the others on their own channel,
 * which only carries conversations they may see (CF-44).
 */
export function useInboxLive(
  activeUuid: string | null,
  onInbound?: (ev: InboxLiveEvent) => void,
) {
  const { org, orgUuid, canRead, canReadAll } = useInboxScope();
  const { user } = useAuth();
  const userUuid = user?.uuid ?? "";
  const channel = !orgUuid
    ? null
    : canReadAll
      ? RealtimeChannels.inbox(orgUuid)
      : userUuid
        ? RealtimeChannels.inboxUser(orgUuid, userUuid)
        : null;
  const qc = useQueryClient();
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const inboundRef = useRef(onInbound);
  useEffect(() => {
    inboundRef.current = onInbound;
  }, [onInbound]);

  const refreshLists = useCallback(() => {
    if (timer.current) clearTimeout(timer.current);
    timer.current = setTimeout(() => {
      void qc.invalidateQueries({ queryKey: inboxKeys.lists(org) });
      void qc.invalidateQueries({ queryKey: inboxKeys.counts(org) });
    }, 250);
  }, [org, qc]);

  useEffect(
    () => () => {
      if (timer.current) clearTimeout(timer.current);
    },
    [],
  );

  useChannel(
    channel,
    (raw) => {
      // The channel manager splits the payload into { type, data }.
      const ev = { ...raw.data, type: raw.type } as unknown as InboxLiveEvent;
      if (!ev?.type || !ev.conversation_uuid) return;
      // An AI agent job finished (CF-246): the composer shows its draft
      // without waiting for a reload. Lists and badges are unchanged.
      if (ev.type === "ai.job_finished") {
        void qc.invalidateQueries({
          queryKey: inboxKeys.aiJobs(org, ev.conversation_uuid),
        });
        return;
      }
      // Read markers are per member (CF-247): only the member who marked
      // read / unread refetches (its other tabs and devices). The open
      // detail stays, or opening it would mark it read again.
      if (ev.type === "conversation.read_state") {
        if (ev.actor_uuid && ev.actor_uuid === userUuid) refreshLists();
        return;
      }
      if (ev.type === "conversation.viewers") {
        qc.setQueryData(
          [...inboxKeys.all(org), "viewers", ev.conversation_uuid],
          ev.viewers ?? [],
        );
        return;
      }
      if (ev.message) {
        upsertMessage(
          qc,
          inboxKeys.messages(org, ev.conversation_uuid),
          ev.message,
        );
      }
      // A new message moves first response, waiting time and the message
      // count of the detail too (only an open detail refetches).
      if (
        ev.type === "conversation.updated" ||
        ev.type === "conversation.created" ||
        ev.type === "message.created"
      ) {
        void qc.invalidateQueries({
          queryKey: inboxKeys.detail(org, ev.conversation_uuid),
        });
      }
      if (
        ev.conversation_uuid === activeUuid &&
        ev.message?.direction === "in"
      ) {
        inboundRef.current?.(ev);
      }
      refreshLists();
    },
    { enabled: Boolean(channel) && canRead },
  );
}

const HEARTBEAT_MS = 10_000;

/**
 * Tells the team this agent has the conversation open (and whether they
 * are typing) and returns the other viewers (CF-49). Heartbeats every 10s
 * and on typing changes; leaves on close.
 */
export function useConversationPresence(uuid: string | null, typing: boolean) {
  const { org } = useInboxScope();
  const { user } = useAuth();
  const qc = useQueryClient();
  const key = useMemo(
    () => [...inboxKeys.all(org), "viewers", uuid ?? ""],
    [org, uuid],
  );
  const viewers = useQuery({
    queryKey: key,
    queryFn: () => inboxService.presence(uuid as string, false),
    enabled: Boolean(org && uuid),
    staleTime: Infinity,
  });
  useEffect(() => {
    if (!org || !uuid) return;
    const beat = () =>
      void inboxService
        .presence(uuid, typing)
        .then((v) => qc.setQueryData(key, v))
        .catch(() => undefined);
    beat();
    const id = setInterval(beat, HEARTBEAT_MS);
    return () => clearInterval(id);
  }, [org, uuid, typing, qc, key]);
  useEffect(() => {
    if (!org || !uuid) return;
    return () => void inboxService.leave(uuid).catch(() => undefined);
  }, [org, uuid]);
  const others = (viewers.data ?? []).filter((v) => v.uuid !== user?.uuid);
  return others;
}

/** Saved inbox views of the caller (CF-47). */
export function useConversationViews() {
  const { org, canRead } = useInboxScope();
  return useQuery({
    queryKey: [...inboxKeys.all(org), "views"],
    queryFn: () => inboxService.views(),
    enabled: Boolean(org) && canRead,
    staleTime: 60_000,
  });
}

/** Open conversation counts per saved view (badges). */
export function useConversationViewCounts() {
  const { org, canRead } = useInboxScope();
  return useQuery({
    queryKey: [...inboxKeys.all(org), "view-counts"],
    queryFn: () => inboxService.viewCounts(),
    enabled: Boolean(org) && canRead,
    staleTime: 30_000,
  });
}

export function useConversationViewMutations() {
  const { org } = useInboxScope();
  const qc = useQueryClient();
  const done = () => {
    void qc.invalidateQueries({ queryKey: [...inboxKeys.all(org), "views"] });
    void qc.invalidateQueries({
      queryKey: [...inboxKeys.all(org), "view-counts"],
    });
  };
  return {
    create: useAppMutation({
      mutationFn: (b: ConversationViewInput) => inboxService.createView(b),
      onSuccess: done,
    }),
    remove: useAppMutation({
      mutationFn: (uuid: string) => inboxService.deleteView(uuid),
      onSuccess: done,
    }),
  };
}
