"use client";

import { createNavAdornment, type NavAdornment } from "@/features/nav-engine";

import { useInboxCounts } from "../hooks/use-inbox";

function useInboxCount(key: "open" | "mine" | "unassigned"): NavAdornment {
  const { data } = useInboxCounts();
  const value = data?.[key] ?? 0;
  return {
    badges:
      value > 0
        ? [
            {
              kind: "count",
              value,
              variant: key === "unassigned" ? "warning" : "secondary",
            },
          ]
        : [],
  };
}

function useOpenAdornment() {
  return useInboxCount("open");
}
function useMineAdornment() {
  return useInboxCount("mine");
}
function useUnassignedAdornment() {
  return useInboxCount("unassigned");
}

/** Live inbox counts on the sidebar items. */
export const inboxNavAdornments = {
  all: createNavAdornment(useOpenAdornment),
  mine: createNavAdornment(useMineAdornment),
  unassigned: createNavAdornment(useUnassignedAdornment),
};
