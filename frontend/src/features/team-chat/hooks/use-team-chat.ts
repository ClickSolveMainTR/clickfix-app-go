"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useCallback, useEffect, useMemo } from "react";

import { Permission } from "@/config/permissions";
import { useActiveOrganization } from "@/features/organizations";
import { useConnectionState, useRealtimeEvent } from "@/hooks/use-realtime";
import { useAppMutation } from "@/lib/query/mutation";

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

/** Messages of a thread, oldest first (the API returns the newest page). */
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
  const items = useMemo(
    () =>
      [...(query.data ?? [])].sort((a, b) =>
        a.created_at.localeCompare(b.created_at),
      ),
    [query.data],
  );
  return { ...query, items };
}

/**
 * Live team chat: the backend publishes `teamchat.changed` with the thread
 * uuid on the member's `user:{uuid}` channel (a new thread, a new message, a
 * new member). RealtimeProvider already subscribes to that channel, so this
 * only listens on the shared dispatcher and refetches what changed.
 */
export function useTeamChatLive() {
  const { org, canUse } = useTeamChatScope();
  const qc = useQueryClient();
  const onEvent = useCallback(
    (message: { data: Record<string, unknown> }) => {
      void qc.invalidateQueries({ queryKey: teamChatKeys.threads(org) });
      const thread = message.data.thread_uuid;
      if (typeof thread === "string") {
        void qc.invalidateQueries({
          queryKey: teamChatKeys.messages(org, thread),
        });
      }
    },
    [org, qc],
  );
  useRealtimeEvent("teamchat.changed", onEvent, {
    enabled: Boolean(org) && canUse,
  });
}

function appendMessage(cur: ThreadMessage[] | undefined, m: ThreadMessage) {
  return cur && !cur.some((x) => x.uuid === m.uuid) ? [...cur, m] : cur;
}

export function useSendThreadMessage(uuid: string) {
  const { org } = useTeamChatScope();
  const qc = useQueryClient();
  return useAppMutation({
    mutationFn: (body: string) => teamChatService.send(uuid, body),
    onSuccess: (m) =>
      qc.setQueryData<ThreadMessage[]>(
        teamChatKeys.messages(org, uuid),
        (cur) => appendMessage(cur, m),
      ),
    onSettled: () =>
      void qc.invalidateQueries({ queryKey: teamChatKeys.threads(org) }),
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
 * open, and clears its unread count in the cached list right away.
 */
export function useMarkThreadRead(uuid: string, newest: string | undefined) {
  const { org } = useTeamChatScope();
  const qc = useQueryClient();
  useEffect(() => {
    if (!org || !uuid || !newest) return;
    let cancelled = false;
    teamChatService
      .markRead(uuid)
      .then(() => {
        if (cancelled) return;
        qc.setQueryData<{ items: Thread[]; total: number }>(
          teamChatKeys.threads(org),
          (cur) =>
            cur && {
              ...cur,
              items: cur.items.map((th) =>
                th.uuid === uuid ? { ...th, unread: 0 } : th,
              ),
            },
        );
      })
      // Best effort: the next message or reload marks it read again.
      .catch(() => undefined);
    return () => {
      cancelled = true;
    };
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
