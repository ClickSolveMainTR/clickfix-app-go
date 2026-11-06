"use client";

import { createNavAdornment, type NavAdornment } from "@/features/nav-engine";

import { useTeamChatLive, useTeamChatUnread } from "../hooks/use-team-chat";

function useTeamChatNavAdornment(): NavAdornment {
  // The sidebar is always mounted, so the badge stays live on every page.
  useTeamChatLive();
  const unread = useTeamChatUnread();
  return {
    badges: unread > 0 ? [{ kind: "count", value: unread }] : [],
  };
}

/** Unread team chat messages on the sidebar item. */
export const TeamChatNavAdornment = createNavAdornment(useTeamChatNavAdornment);
