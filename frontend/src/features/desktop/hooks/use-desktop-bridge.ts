"use client";

import { useQuery } from "@tanstack/react-query";
import { useEffect, useRef } from "react";

import { inboxKeys, useInboxScope } from "@/features/inbox/hooks/use-inbox";
import { inboxService } from "@/features/inbox/services/inbox.service";
import type { PresenceStatus } from "@/features/presence";
import {
  isDesktop,
  onDesktopCommand,
  setBadge,
  setPresence,
} from "@/lib/desktop/bridge";

/**
 * ClickFix Masaüstü badge (CF-236): open conversations assigned to the
 * caller with unread messages. Only runs inside the desktop app; the count
 * is the list total of `assignee=me&status=open,pending&unread=1` (one row
 * fetched). It sits under the inbox list key, so inbox live updates refresh
 * it too.
 */
export function useDesktopBadge(enabled = true) {
  const { org, canRead } = useInboxScope();
  const desktop = isDesktop();
  const query = useQuery({
    queryKey: [...inboxKeys.lists(org), "desktop-badge"] as const,
    queryFn: async () => {
      const page = await inboxService.list({
        view: "mine",
        status: "open",
        q: "",
        unread: true,
        sort: "-last_message_at",
        limit: 1,
        offset: 0,
      });
      return page.total ?? 0;
    },
    enabled: enabled && desktop && Boolean(org) && canRead,
    staleTime: 15_000,
    refetchInterval: 60_000,
    refetchIntervalInBackground: true,
  });
  const count = desktop && org && canRead ? (query.data ?? 0) : 0;

  useEffect(() => {
    if (!desktop) return;
    void setBadge(count);
  }, [desktop, count]);
}

/**
 * Keeps the tray menu's status in step with the web status menu and applies
 * a status picked in the tray. No-op outside the desktop app.
 */
export function useDesktopPresence(
  status: PresenceStatus | null,
  setStatus: (status: PresenceStatus) => void,
) {
  const setRef = useRef(setStatus);
  useEffect(() => {
    setRef.current = setStatus;
  }, [setStatus]);

  useEffect(() => {
    if (!isDesktop()) return;
    void setPresence(status);
  }, [status]);

  useEffect(
    () =>
      onDesktopCommand((command) => {
        if (command.type === "presence") setRef.current(command.status);
      }),
    [],
  );
}

/** Tray "Oturumu kapat": signs the web view out as well. */
export function useDesktopSignOut(logout: () => Promise<void>) {
  const logoutRef = useRef(logout);
  useEffect(() => {
    logoutRef.current = logout;
  }, [logout]);

  useEffect(
    () =>
      onDesktopCommand((command) => {
        if (command.type === "sign-out") void logoutRef.current();
      }),
    [],
  );
}
