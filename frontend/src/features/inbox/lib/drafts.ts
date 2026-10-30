"use client";

import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";

export type Draft = { text: string; mode: "reply" | "note"; at: number };

type DraftState = {
  drafts: Record<string, Draft>;
  set: (conversation: string, draft: Omit<Draft, "at">) => void;
  clear: (conversation: string) => void;
};

/** Keep drafts for a week; the store never grows past this many entries. */
const MAX_AGE_MS = 7 * 24 * 60 * 60 * 1000;
const MAX_DRAFTS = 200;

/** Composer drafts per conversation, persisted in this browser. */
export const useDrafts = create<DraftState>()(
  persist(
    (set) => ({
      drafts: {},
      set: (conversation, draft) =>
        set((s) => {
          const drafts = { ...s.drafts };
          if (!draft.text.trim() && draft.mode === "reply") {
            delete drafts[conversation];
          } else {
            drafts[conversation] = { ...draft, at: Date.now() };
          }
          return { drafts: prune(drafts) };
        }),
      clear: (conversation) =>
        set((s) => {
          const drafts = { ...s.drafts };
          delete drafts[conversation];
          return { drafts };
        }),
    }),
    {
      name: "clickfix.inbox.drafts",
      storage: createJSONStorage(() => localStorage),
      partialize: (s) => ({ drafts: s.drafts }),
    },
  ),
);

function prune(drafts: Record<string, Draft>): Record<string, Draft> {
  const now = Date.now();
  const entries = Object.entries(drafts)
    .filter(([, d]) => now - d.at < MAX_AGE_MS)
    .sort((a, b) => b[1].at - a[1].at)
    .slice(0, MAX_DRAFTS);
  return Object.fromEntries(entries);
}
