"use client";

import { useEffect } from "react";
import type { LucideIcon } from "lucide-react";
import { create } from "zustand";

/**
 * Palette actions (CF-50): screens register what can be done right now
 * (resolve this conversation, apply a macro, go to a view) and the Cmd+K
 * palette lists them above search results. Recently run actions float up.
 */
export type PaletteAction = {
  /** Stable id (recent list key), e.g. "inbox.resolve". */
  id: string;
  label: string;
  description?: string;
  /** Group heading. */
  group: string;
  icon: LucideIcon;
  /** Extra words matched by the filter. */
  keywords?: string;
  run: () => void;
};

type State = {
  sources: Record<string, PaletteAction[]>;
  set: (source: string, actions: PaletteAction[]) => void;
  clear: (source: string) => void;
};

export const usePaletteActionStore = create<State>()((set) => ({
  sources: {},
  set: (source, actions) =>
    set((s) => ({ sources: { ...s.sources, [source]: actions } })),
  clear: (source) =>
    set((s) => {
      const sources = { ...s.sources };
      delete sources[source];
      return { sources };
    }),
}));

/** Registers a screen's actions while it is mounted. */
export function usePaletteActions(source: string, actions: PaletteAction[]) {
  const setActions = usePaletteActionStore((s) => s.set);
  const clear = usePaletteActionStore((s) => s.clear);
  useEffect(() => {
    setActions(source, actions);
  }, [source, actions, setActions]);
  useEffect(() => () => clear(source), [source, clear]);
}

const RECENT_KEY = "app.palette.actions.recent.v1";
const MAX_RECENT = 6;

export function readRecentActions(): string[] {
  if (typeof window === "undefined") return [];
  try {
    const raw = window.localStorage.getItem(RECENT_KEY);
    const parsed = raw ? (JSON.parse(raw) as unknown) : [];
    return Array.isArray(parsed)
      ? parsed.filter((x): x is string => typeof x === "string")
      : [];
  } catch {
    return [];
  }
}

export function pushRecentAction(id: string) {
  if (typeof window === "undefined") return;
  try {
    const next = [id, ...readRecentActions().filter((x) => x !== id)].slice(
      0,
      MAX_RECENT,
    );
    window.localStorage.setItem(RECENT_KEY, JSON.stringify(next));
  } catch {
    // Storage may be unavailable (private mode); recents are a convenience.
  }
}

/** Actions matching query (all words, case and Turkish-dotless insensitive), recent first. */
export function filterActions(
  actions: PaletteAction[],
  query: string,
  recent: string[],
): PaletteAction[] {
  const norm = (s: string) =>
    s
      .toLocaleLowerCase("tr")
      .normalize("NFKD")
      .replace(/[̀-ͯ]/g, "")
      .replace(/ı/g, "i");
  const words = norm(query).split(/\s+/).filter(Boolean);
  const matched = actions.filter((a) => {
    const hay = norm(
      `${a.label} ${a.description ?? ""} ${a.group} ${a.keywords ?? ""}`,
    );
    return words.every((w) => hay.includes(w));
  });
  const rank = (a: PaletteAction) => {
    const i = recent.indexOf(a.id);
    return i < 0 ? recent.length : i;
  };
  return [...matched].sort((a, b) => rank(a) - rank(b));
}
