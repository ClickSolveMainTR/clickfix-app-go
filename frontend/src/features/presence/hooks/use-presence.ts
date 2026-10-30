"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import { useActiveOrganization } from "@/features/organizations";
import {
  presenceService,
  type PresenceMe,
  type PresenceMember,
  type PresenceStatus,
} from "@/features/presence/services/presence.service";
import {
  applyTypingEvent,
  pruneTyping,
  TYPING_IDLE_MS,
  TYPING_SEND_INTERVAL_MS,
  TYPING_TTL_MS,
  type TypingEntry,
} from "@/features/presence/lib/typing";
import { useChannel, usePresence } from "@/hooks/use-realtime";
import { RealtimeChannels } from "@/lib/realtime";
import { useAuth } from "@/providers/auth-provider";

export const presenceKeys = {
  team: (org: string) => ["presence", org, "team"] as const,
  me: (org: string) => ["presence", org, "me"] as const,
};

const HEARTBEAT_FALLBACK_MS = 30_000;

/**
 * Members of the active organization with their status. Loaded once, then
 * kept live from `presence.changed` on `org:{uuid}`.
 */
export function useTeamPresence() {
  const { active } = useActiveOrganization();
  const queryClient = useQueryClient();
  const org = active?.slug ?? "";
  const query = useQuery({
    queryKey: presenceKeys.team(org),
    queryFn: () => presenceService.list(),
    enabled: !!org,
    staleTime: 60_000,
  });

  useChannel(active ? RealtimeChannels.org(active.uuid) : null, (message) => {
    if (message.type !== "presence.changed") return;
    const data = message.data as {
      user_uuid?: string;
      status?: PresenceStatus;
      last_seen_at?: string;
    };
    if (!data.user_uuid || !data.status) return;
    queryClient.setQueryData<PresenceMember[]>(presenceKeys.team(org), (prev) =>
      prev?.map((m) =>
        m.user_uuid === data.user_uuid
          ? {
              ...m,
              status: data.status!,
              last_seen_at: data.last_seen_at ?? m.last_seen_at,
              changed_at: new Date().toISOString(),
            }
          : m,
      ),
    );
  });

  return query;
}

/**
 * The signed-in agent's status in the active organization: heartbeats while
 * the tab is visible (the server turns silent agents offline) and exposes
 * setStatus for the status menu.
 */
export function useMyPresence(options?: { enabled?: boolean }) {
  const { active } = useActiveOrganization();
  const queryClient = useQueryClient();
  const org = active?.slug ?? "";
  const enabled = (options?.enabled ?? true) && !!org;

  const me = useQuery({
    queryKey: presenceKeys.me(org),
    queryFn: () => presenceService.heartbeat(),
    enabled,
    refetchInterval: (q) =>
      ((q.state.data as PresenceMe | undefined)?.heartbeat_seconds ?? 0) *
        1000 || HEARTBEAT_FALLBACK_MS,
    refetchIntervalInBackground: false,
    refetchOnWindowFocus: true,
  });

  const setStatus = useMutation({
    mutationFn: (status: PresenceStatus) => presenceService.setStatus(status),
    onSuccess: (next) => {
      queryClient.setQueryData(presenceKeys.me(org), next);
      void queryClient.invalidateQueries({
        queryKey: presenceKeys.team(org),
      });
    },
  });

  useAutoAway(enabled, (next) =>
    queryClient.setQueryData(presenceKeys.me(org), next),
  );

  return {
    status: me.data?.status ?? null,
    isLoading: me.isLoading,
    setStatus: setStatus.mutate,
    isSaving: setStatus.isPending,
  };
}

export type ConversationViewer = { userUuid: string; name: string };

/**
 * Agents who currently have a conversation open (Centrifugo presence on
 * `conversation:{uuid}`; names come from the connection info).
 */
export function useConversationViewers(conversationUuid: string | null) {
  const { user } = useAuth();
  const channel = conversationUuid
    ? RealtimeChannels.conversation(conversationUuid)
    : null;
  const { clients } = usePresence(channel);
  return useMemo(() => {
    const seen = new Map<string, ConversationViewer>();
    for (const client of Object.values(clients)) {
      if (!client.user || client.user === user?.uuid) continue;
      const info = client.connInfo as { name?: unknown } | undefined;
      seen.set(client.user, {
        userUuid: client.user,
        name: typeof info?.name === "string" ? info.name : "",
      });
    }
    return [...seen.values()];
  }, [clients, user?.uuid]);
}

/**
 * Typing indicators of a conversation: `typing` lists the other agents
 * typing now; call `notifyTyping()` on every keystroke (sends are throttled
 * and a "stop" follows a short pause).
 */
export function useTyping(conversationUuid: string | null) {
  const { user } = useAuth();
  const selfUuid = user?.uuid ?? null;
  const [typing, setTyping] = useState<TypingEntry[]>([]);
  const lastSent = useRef(0);
  const stopTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  useChannel(
    conversationUuid ? RealtimeChannels.conversation(conversationUuid) : null,
    (message) => {
      if (message.type !== "conversation.typing") return;
      setTyping((prev) =>
        applyTypingEvent(prev, message.data, Date.now(), selfUuid),
      );
    },
  );

  useEffect(() => {
    if (typing.length === 0) return;
    const timer = setTimeout(
      () => setTyping((prev) => pruneTyping(prev, Date.now())),
      TYPING_TTL_MS,
    );
    return () => clearTimeout(timer);
  }, [typing]);

  const send = useCallback(
    (state: "start" | "stop") => {
      if (!conversationUuid) return;
      // Typing is best effort: a 429 or network error is not worth a toast.
      presenceService.typing(conversationUuid, state).catch(() => undefined);
    },
    [conversationUuid],
  );

  const notifyTyping = useCallback(() => {
    const now = Date.now();
    if (now - lastSent.current >= TYPING_SEND_INTERVAL_MS) {
      lastSent.current = now;
      send("start");
    }
    if (stopTimer.current) clearTimeout(stopTimer.current);
    stopTimer.current = setTimeout(() => {
      lastSent.current = 0;
      send("stop");
    }, TYPING_IDLE_MS);
  }, [send]);

  useEffect(
    () => () => {
      if (stopTimer.current) clearTimeout(stopTimer.current);
    },
    [conversationUuid],
  );

  return { typing, notifyTyping };
}

/** Minutes without keyboard, mouse or touch before an agent shows as away. */
export const AUTO_AWAY_MS = 10 * 60_000;

/**
 * Automatic away (CF-54): after AUTO_AWAY_MS without input the agent is
 * reported idle (the server moves "online" to "away", keeping the status they
 * picked); the next input reports them back.
 */
function useAutoAway(enabled: boolean, onChange: (me: PresenceMe) => void) {
  const idle = useRef(false);
  const last = useRef(0);
  const report = useRef(onChange);
  useEffect(() => {
    report.current = onChange;
  });
  useEffect(() => {
    if (!enabled) return;
    last.current = Date.now();
    const send = (next: boolean) => {
      idle.current = next;
      presenceService
        .idle(next)
        .then((me) => report.current(me))
        .catch(() => undefined);
    };
    const onInput = () => {
      last.current = Date.now();
      if (idle.current) send(false);
    };
    const events = [
      "mousemove",
      "keydown",
      "pointerdown",
      "touchstart",
      "scroll",
    ];
    for (const e of events)
      window.addEventListener(e, onInput, { passive: true });
    const timer = window.setInterval(() => {
      if (!idle.current && Date.now() - last.current >= AUTO_AWAY_MS)
        send(true);
    }, 30_000);
    return () => {
      for (const e of events) window.removeEventListener(e, onInput);
      window.clearInterval(timer);
    };
  }, [enabled]);
}
