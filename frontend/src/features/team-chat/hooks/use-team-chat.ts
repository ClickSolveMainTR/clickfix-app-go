"use client";

import {
  useQuery,
  useQueryClient,
  type QueryClient,
} from "@tanstack/react-query";
import { useEffect, useMemo } from "react";

import { Permission } from "@/config/permissions";
import { useActiveOrganization } from "@/features/organizations";
import { useConnectionState } from "@/hooks/use-realtime";
import { useAppMutation } from "@/lib/query/mutation";
import { realtimeManager } from "@/lib/realtime";
import { useAuth } from "@/providers/auth-provider";

import {
  teamChatService,
  type Thread,
  type ThreadInput,
  type ThreadMessage,
} from "../services/team-chat.service";

export const teamChatKeys = {
  all: (org: string) => ["team-chat", org] as const,
  threads: (org: string) => [...teamChatKeys.all(org), "threads"] as const,
  messages: (org: string, uuid: string) =>
    [...teamChatKeys.all(org), "messages", uuid] as const,
};

/** Polling of the mobile app; on web only while realtime is not connected. */
const THREADS_POLL_MS = 30_000;
const MESSAGES_POLL_MS = 10_000;

/** The active organization and whether the caller may use team chat. */
export function useTeamChatScope() {
  const { active } = useActiveOrganization();
  return {
    org: active?.slug ?? "",
    canUse: (active?.permissions ?? []).includes(Permission.TenantTeamChatUse),
  };
}

function useLive() {
  return useConnectionState() === "connected";
}

export function useThreads() {
  const { org, canUse } = useTeamChatScope();
  const live = useLive();
  return useQuery({
    queryKey: teamChatKeys.threads(org),
    queryFn: () => teamChatService.threads(),
    enabled: Boolean(org) && canUse,
    staleTime: 15_000,
    refetchInterval: live ? false : THREADS_POLL_MS,
  });
}

/** Unread team chat messages over every thread (the sidebar badge). */
export function useTeamChatUnread() {
  const threads = useThreads();
  return (threads.data?.items ?? []).reduce((n, th) => n + (th.unread ?? 0), 0);
}

/**
 * Messages of a thread, oldest first. The cache keeps the API order (newest
 * first, by id), so a sent message is prepended and the view reverses it.
 */
export function useThreadMessages(uuid: string | null) {
  const { org, canUse } = useTeamChatScope();
  const live = useLive();
  const query = useQuery({
    queryKey: teamChatKeys.messages(org, uuid ?? ""),
    queryFn: () => teamChatService.messages(uuid as string),
    enabled: Boolean(org && uuid) && canUse,
    staleTime: 5_000,
    refetchInterval: live ? false : MESSAGES_POLL_MS,
  });
  const items = useMemo(() => [...(query.data ?? [])].reverse(), [query.data]);
  return { ...query, items };
}

// One dispatcher listener however many components ask for live updates (the
// sidebar badge and the page are mounted together), so an event invalidates
// each query once.
const shared = {
  users: 0,
  off: null as (() => void) | null,
  org: "",
  qc: null as QueryClient | null,
};

/**
 * Live team chat: the backend publishes `teamchat.changed` with the thread
 * uuid on the member's `user:{uuid}` channel (a new thread, a new message, a
 * new member). RealtimeProvider already subscribes to that channel, so this
 * only listens on the shared dispatcher and refetches what changed.
 */
export function useTeamChatLive() {
  const { org, canUse } = useTeamChatScope();
  const qc = useQueryClient();
  const enabled = Boolean(org) && canUse;
  useEffect(() => {
    if (!enabled) return;
    shared.org = org;
    shared.qc = qc;
    shared.users += 1;
    shared.off ??= realtimeManager.onEvent("teamchat.changed", (message) => {
      if (!shared.qc) return;
      void shared.qc.invalidateQueries({
        queryKey: teamChatKeys.threads(shared.org),
      });
      const thread = message.data.thread_uuid;
      if (typeof thread === "string") {
        void shared.qc.invalidateQueries({
          queryKey: teamChatKeys.messages(shared.org, thread),
        });
      }
    });
    return () => {
      shared.users -= 1;
      if (shared.users === 0) {
        shared.off?.();
        shared.off = null;
      }
    };
  }, [enabled, org, qc]);
}

export function useSendThreadMessage(uuid: string) {
  const { org } = useTeamChatScope();
  const { user } = useAuth();
  const qc = useQueryClient();
  const key = teamChatKeys.messages(org, uuid);
  return useAppMutation({
    mutationFn: (v: { body: string; mentions: string[] }) =>
      teamChatService.send(uuid, v.body, v.mentions),
    // The POST answer has no `sender_uuid` (the list does), so the sender is
    // filled in here; otherwise the bubble would show as someone else's.
    onSuccess: (m) =>
      qc.setQueryData<ThreadMessage[]>(key, (cur) =>
        cur && !cur.some((x) => x.uuid === m.uuid)
          ? [{ ...m, sender_uuid: m.sender_uuid ?? user?.uuid }, ...cur]
          : cur,
      ),
    // The thread list is refreshed by useMarkThreadRead once the new message
    // is marked read: refreshing it here could land before that and leave the
    // sender's own message counted as unread.
    onSettled: () => void qc.invalidateQueries({ queryKey: key }),
  });
}

export function useAskAi(uuid: string) {
  const { org } = useTeamChatScope();
  const qc = useQueryClient();
  return useAppMutation({
    mutationFn: (question: string) => teamChatService.askAi(uuid, question),
    onSettled: () =>
      void qc.invalidateQueries({
        queryKey: teamChatKeys.messages(org, uuid),
      }),
  });
}

/**
 * Marks the thread read whenever its newest message changes while it is
 * open. Once the server has it, the unread count is cleared in the cached
 * list and the list is refetched: a list request that started earlier is
 * cancelled, so the count read after the mark always wins. This also runs
 * when the user has moved on to another thread in the meantime.
 */
export function useMarkThreadRead(uuid: string, newest: string | undefined) {
  const { org } = useTeamChatScope();
  const qc = useQueryClient();
  useEffect(() => {
    if (!org || !uuid || !newest) return;
    const key = teamChatKeys.threads(org);
    teamChatService
      .markRead(uuid)
      .then(() => {
        qc.setQueryData<{ items: Thread[]; total: number }>(
          key,
          (cur) =>
            cur && {
              ...cur,
              items: cur.items.map((th) =>
                th.uuid === uuid ? { ...th, unread: 0 } : th,
              ),
            },
        );
        return qc.invalidateQueries({ queryKey: key });
      })
      // Best effort: the next message or reload marks it read again.
      .catch(() => undefined);
  }, [org, uuid, newest, qc]);
}

export function useCreateThread() {
  const { org } = useTeamChatScope();
  const qc = useQueryClient();
  return useAppMutation({
    mutationFn: (body: ThreadInput) => teamChatService.create(body),
    onSettled: () =>
      void qc.invalidateQueries({ queryKey: teamChatKeys.threads(org) }),
  });
}
