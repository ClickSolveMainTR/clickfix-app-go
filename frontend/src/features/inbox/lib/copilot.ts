"use client";

import { useEffect, useState } from "react";

import { isApiError } from "@/lib/api";
import { usePrefersReducedMotion } from "@/lib/motion";

import type { AICitation, CopilotOperation } from "../services/copilot.service";

/**
 * What a copilot result is for: `draft` results replace or extend the reply
 * the agent is writing, `insight` results (summary, next step, answers) are
 * read in the panel and can be kept as an internal note.
 */
export type CopilotKind = "draft" | "insight";

/** Operations that rework the text already in the composer. */
export const DRAFT_TRANSFORMS = [
  "rewrite",
  "shorten",
  "expand",
  "tone",
  "translate",
] as const satisfies readonly CopilotOperation[];

export function copilotKind(op: CopilotOperation): CopilotKind {
  return op === "summarize" || op === "next_action" || op === "ask"
    ? "insight"
    : "draft";
}

/** Operations that need the agent's draft as input. */
export function needsDraft(op: CopilotOperation): boolean {
  return (DRAFT_TRANSFORMS as readonly string[]).includes(op);
}

/** Tones the agent can pick; sent as the free-form `tone` hint. */
export const COPILOT_TONES = [
  "friendly",
  "formal",
  "empathetic",
  "concise",
] as const;
export type CopilotTone = (typeof COPILOT_TONES)[number];

/** Target languages offered for "translate" (backend accepts any `xx`). */
export const COPILOT_LANGUAGES = [
  "tr",
  "en",
  "de",
  "fr",
  "es",
  "ar",
  "ru",
] as const;

/**
 * Keyboard map of the copilot. Global keys work like the other inbox keys
 * (off while typing); `⌘ .` works inside the composer.
 */
export const COPILOT_SHORTCUTS = [
  { keys: ["G"], labelKey: "inbox.shortcuts.copilot_suggest" },
  { keys: ["U"], labelKey: "inbox.shortcuts.copilot_summarize" },
  { keys: ["Q"], labelKey: "inbox.shortcuts.copilot_ask" },
  { keys: ["⌘", "."], labelKey: "inbox.shortcuts.copilot_menu" },
] as const;

const UUID = "[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}";

/**
 * The model is told to cite knowledge chunks by id. Ids mean nothing to the
 * agent, so each cited chunk becomes a numbered marker (`[1]`) that matches
 * the source chips. Unknown ids (hallucinated or not retrieved) are dropped
 * instead of being shown as sources.
 */
export function numberCitations(
  text: string,
  citations: AICitation[],
): { text: string; sources: AICitation[] } {
  const order: string[] = [];
  const known = new Map(citations.map((c) => [c.chunk_uuid.toLowerCase(), c]));
  const pattern = new RegExp(
    String.raw`\s*[\[(]?(?:(?:chunk|source|kaynak)\s*[:=]?\s*)?(${UUID})[\])]?`,
    "gi",
  );
  const out = text
    .replace(pattern, (_m, id: string) => {
      const key = id.toLowerCase();
      if (!known.has(key)) return "";
      let n = order.indexOf(key);
      if (n === -1) n = order.push(key) - 1;
      return `[${n + 1}]`;
    })
    // Lines that only listed sources ("Kaynaklar: [1] [2]") add nothing.
    .replace(/^\s*(?:sources?|kaynaklar?)\s*:?\s*(?:\[\d+\]\s*,?\s*)*$/gim, "")
    .replace(/(\[\d+\])(?:\s*,?\s*\1)+/g, "$1")
    .replace(/\n{3,}/g, "\n\n")
    .trim();
  const cited = order.map((k) => known.get(k) as AICitation);
  // Grounded on retrieval even when the model did not echo the ids.
  const sources =
    cited.length > 0
      ? cited
      : citations.filter(
          (c, i, all) =>
            all.findIndex((x) => x.article_uuid === c.article_uuid) === i,
        );
  return { text: out, sources };
}

/** Text ready for the composer: numbered markers removed. */
export function stripMarkers(text: string): string {
  return text
    .replace(/\s?\[\d+\]/g, "")
    .replace(/[ \t]+\n/g, "\n")
    .trim();
}

/**
 * Progressive reveal ("streaming feel") of a finished answer. The API returns
 * the whole text at once; revealing it over ~0.9s keeps the agent's eye on
 * the words as they land. Off for reduced motion.
 */
export function useTypewriter(text: string, key: string): string {
  const reduced = usePrefersReducedMotion();
  const [state, setState] = useState({ key, shown: reduced ? text.length : 0 });
  if (state.key !== key) setState({ key, shown: reduced ? text.length : 0 });

  useEffect(() => {
    if (reduced) return;
    let frame = 0;
    const started = performance.now();
    const duration = Math.min(1400, 350 + text.length * 2);
    const tick = (now: number) => {
      const p = Math.min(1, (now - started) / duration);
      // Ease-out so the tail settles instead of stopping abruptly.
      const eased = 1 - (1 - p) ** 3;
      setState((s) =>
        s.key === key ? { key, shown: Math.ceil(text.length * eased) } : s,
      );
      if (p < 1) frame = requestAnimationFrame(tick);
    };
    frame = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(frame);
  }, [text, key, reduced]);

  return text.slice(0, state.key === key ? state.shown : 0);
}

/**
 * i18n key for an AI error code; the same codes are the `reason` of
 * GET /v1/tenant/ai/availability.
 */
export function aiCodeKey(code: string | undefined): string | undefined {
  switch (code) {
    case "AI_CREDITS_EXHAUSTED":
      return "inbox.copilot.error.credits";
    case "AI_LIMIT_REACHED":
      return "inbox.copilot.error.limit";
    case "AI_GUARD_REJECTED":
      return "inbox.copilot.error.guard";
    case "AI_PROVIDER_UNAVAILABLE":
      return "inbox.copilot.error.provider";
    case "AI_DISABLED":
      return "inbox.copilot.error.disabled";
    case "AI_NOT_CONFIGURED":
      return "inbox.copilot.error.not_configured";
    case "AI_NOT_ENTITLED":
      return "inbox.copilot.error.not_entitled";
  }
  return undefined;
}

/** i18n key of an inline error for a failed copilot call. */
export function copilotErrorKey(error: unknown): string {
  if (!isApiError(error)) return "inbox.copilot.error.generic";
  const key = aiCodeKey(error.code);
  if (key) return key;
  if (error.status === 403) return "inbox.copilot.error.disabled";
  if (error.status === 400) return "inbox.copilot.error.invalid";
  return "inbox.copilot.error.generic";
}
