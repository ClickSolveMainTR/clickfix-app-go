"use client";

import { useQuery } from "@tanstack/react-query";
import { useCallback, useEffect, useRef, useState } from "react";

import { useLocale } from "@/providers/locale-provider";

import { numberCitations } from "../lib/copilot";
import { inboxKeys, useInboxScope } from "./use-inbox";
import {
  copilotService,
  type AICitation,
  type CopilotInput,
  type CopilotOperation,
} from "../services/copilot.service";

export type CopilotEntry = {
  id: string;
  op: CopilotOperation;
  /** Draft or question the operation ran on (shown as "original"). */
  input: string;
  /** Tone or target language the agent picked, for the panel title. */
  option?: string;
  /** `agent` = draft produced by the autonomous agent (CF-64), not a call. */
  origin: "copilot" | "agent";
  /**
   * Agent draft of a handoff (CF-246): the model's text without a checked
   * source; the panel asks the agent to verify it before sending.
   */
  unsourced?: boolean;
  status: "pending" | "done" | "error";
  text?: string;
  sources?: AICitation[];
  error?: unknown;
  model?: string;
  tokens?: number;
  credits?: number;
  latencyMs?: number;
  startedAt: number;
};

export type CopilotRequest = {
  op: CopilotOperation;
  text?: string;
  tone?: string;
  locale?: string;
};

const HISTORY = 6;

/**
 * AI availability of the active organization. Copilot actions are disabled
 * (with the reason) while it reports unavailable; a failed check leaves them
 * on, the call itself then explains the error.
 */
export function useAIAvailability(enabled: boolean) {
  const { org } = useInboxScope();
  return useQuery({
    queryKey: inboxKeys.aiAvailability(org),
    queryFn: () => copilotService.availability(org),
    enabled: enabled && Boolean(org),
    staleTime: 60_000,
  });
}

/**
 * Copilot session of one conversation: a short history of results (newest
 * first) the agent can page through, at most one request in flight. The
 * composer is keyed by conversation, so the history resets with it.
 */
export function useCopilot(conversationUuid: string) {
  const { locale } = useLocale();
  const [entries, setEntries] = useState<CopilotEntry[]>([]);
  const [index, setIndex] = useState(0);
  const [open, setOpen] = useState(false);
  const abort = useRef<AbortController | null>(null);

  useEffect(() => () => abort.current?.abort(), []);

  const patch = useCallback(
    (id: string, next: Partial<CopilotEntry>) =>
      setEntries((list) =>
        list.map((e) => (e.id === id ? { ...e, ...next } : e)),
      ),
    [],
  );

  const push = useCallback((entry: CopilotEntry) => {
    setEntries((list) => [entry, ...list].slice(0, HISTORY));
    setIndex(0);
    setOpen(true);
  }, []);

  const run = useCallback(
    (req: CopilotRequest) => {
      abort.current?.abort();
      const controller = new AbortController();
      abort.current = controller;
      const id = crypto.randomUUID();
      const startedAt = performance.now();
      const body: CopilotInput = {
        operation: req.op,
        locale: req.locale ?? (locale === "en" ? "en" : "tr"),
        ...(req.text ? { text: req.text } : {}),
        ...(req.tone ? { tone: req.tone } : {}),
      };
      push({
        id,
        op: req.op,
        input: req.text ?? "",
        option: req.tone ?? (req.op === "translate" ? req.locale : undefined),
        origin: "copilot",
        status: "pending",
        startedAt,
      });
      copilotService
        .run(conversationUuid, body, controller.signal)
        .then((res) => {
          if (controller.signal.aborted) return;
          const { text, sources } = numberCitations(res.text, res.citations);
          patch(id, {
            status: "done",
            text,
            sources,
            model: res.model,
            tokens: res.input_tokens + res.output_tokens,
            credits: res.credits,
            latencyMs: Math.round(performance.now() - startedAt),
          });
        })
        .catch((error: unknown) => {
          if (controller.signal.aborted) return;
          patch(id, { status: "error", error });
        });
    },
    [conversationUuid, locale, patch, push],
  );

  const showAgentDraft = useCallback(
    (jobUuid: string, text: string, unsourced = false) => {
      push({
        id: jobUuid,
        op: "suggest_reply",
        input: "",
        origin: "agent",
        unsourced,
        status: "done",
        text,
        sources: [],
        startedAt: performance.now(),
      });
    },
    [push],
  );

  /** Stops waiting for the answer (the server run still completes). */
  const cancel = useCallback(() => {
    abort.current?.abort();
    setEntries((list) => list.filter((e) => e.status !== "pending"));
    setIndex(0);
  }, []);

  const close = useCallback(() => {
    if (entries[index]?.status === "pending") cancel();
    setOpen(false);
  }, [cancel, entries, index]);

  const current = open ? entries[index] : undefined;
  return {
    current,
    open,
    setOpen,
    entries,
    index,
    setIndex,
    run,
    cancel,
    close,
    showAgentDraft,
    pending: entries.some((e) => e.status === "pending"),
  };
}

export type Copilot = ReturnType<typeof useCopilot>;
