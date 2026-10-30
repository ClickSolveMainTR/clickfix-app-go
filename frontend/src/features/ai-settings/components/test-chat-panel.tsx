"use client";

import { useMutation, useQuery } from "@tanstack/react-query";
import { BookOpen, ExternalLink, Loader2, Send, Sparkles } from "lucide-react";
import Link from "next/link";
import { useEffect, useRef, useState } from "react";

import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Textarea } from "@/components/ui/textarea";
import { routes } from "@/config/routes";
import { useLocale } from "@/providers/locale-provider";

import { aiErrorMessage } from "../lib/errors";
import { formatLatency, formatMicros, formatNumber } from "../lib/format";
import {
  aiSettingsService,
  type AIAgentConfig,
  type AIRunResult,
} from "../services/ai-settings.service";
import { Stat } from "./parts";

/**
 * Reveals text gradually. The run API returns the guarded answer in one
 * piece (output guards run before anything leaves the server), so this only
 * paces the display; reduced motion shows it at once.
 */
function useReveal(text: string) {
  const [state, setState] = useState({ text: "", shown: 0 });
  if (state.text !== text) setState({ text, shown: 0 });
  const reduced =
    typeof window !== "undefined" &&
    window.matchMedia?.("(prefers-reduced-motion: reduce)").matches;
  useEffect(() => {
    if (!text || reduced) return;
    const step = Math.max(2, Math.ceil(text.length / 80));
    const id = window.setInterval(() => {
      setState((prev) => {
        const shown = Math.min(prev.text.length, prev.shown + step);
        if (shown >= prev.text.length) window.clearInterval(id);
        return { ...prev, shown };
      });
    }, 16);
    return () => window.clearInterval(id);
  }, [text, reduced]);
  return reduced ? text : text.slice(0, state.text === text ? state.shown : 0);
}

/**
 * "Test sohbeti" (CF-64): sends one message through the guarded, budgeted
 * `POST /v1/tenant/ai/runs` (task `reply`, knowledge retrieval on) and shows
 * the answer with its citations, tokens, cost and latency. Real runs count
 * against the organization's AI budget and appear in the run log.
 */
export function TestChatPanel({
  slug,
  canRun,
  locale,
}: {
  slug: string;
  canRun: boolean;
  locale: AIAgentConfig["locale"];
}) {
  const { t, locale: uiLocale } = useLocale();
  const [message, setMessage] = useState("");
  const [asked, setAsked] = useState("");
  const startedAt = useRef(0);
  const [elapsed, setElapsed] = useState<number | null>(null);
  const run = useMutation({
    mutationFn: (input: string) =>
      aiSettingsService.execute(slug, {
        task: "reply",
        input,
        locale,
        retrieve: true,
        max_tokens: 1024,
      }),
    onMutate: () => {
      startedAt.current = performance.now();
      setElapsed(null);
    },
    onSettled: () =>
      setElapsed(Math.round(performance.now() - startedAt.current)),
  });
  const result: AIRunResult | undefined = run.data;
  const detail = useQuery({
    queryKey: ["tenant", slug, "ai", "runs", result?.uuid],
    queryFn: () => aiSettingsService.run(slug, result!.uuid),
    enabled: Boolean(result?.uuid),
  });
  const revealed = useReveal(result?.text ?? "");

  const send = () => {
    const input = message.trim();
    if (!input || run.isPending) return;
    setAsked(input);
    setMessage("");
    run.mutate(input);
  };

  return (
    <Card className="xl:sticky xl:top-4">
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <Sparkles className="size-4" />
          {t("ai_settings.test.title")}
        </CardTitle>
        <p className="text-muted-foreground text-xs">
          {t("ai_settings.test.hint")}
        </p>
      </CardHeader>
      <CardContent className="space-y-4">
        {!canRun ? (
          <p className="text-muted-foreground text-sm">
            {t("ai_settings.test.forbidden")}
          </p>
        ) : (
          <>
            <div
              className="bg-muted/30 max-h-[28rem] min-h-40 space-y-3 overflow-y-auto rounded-lg border p-3"
              aria-live="polite"
            >
              {!asked && (
                <p className="text-muted-foreground py-10 text-center text-sm">
                  {t("ai_settings.test.empty")}
                </p>
              )}
              {asked && (
                <div className="flex justify-end">
                  <p className="bg-primary text-primary-foreground max-w-[85%] rounded-lg rounded-br-sm px-3 py-2 text-sm whitespace-pre-wrap">
                    {asked}
                  </p>
                </div>
              )}
              {run.isPending && (
                <p className="text-muted-foreground flex items-center gap-2 text-sm">
                  <Loader2 className="size-4 animate-spin" />
                  {t("ai_settings.test.thinking")}
                </p>
              )}
              {run.isError && (
                <Alert variant="destructive">
                  <AlertDescription>
                    {aiErrorMessage(run.error, t)}
                  </AlertDescription>
                </Alert>
              )}
              {result && (
                <div className="bg-card max-w-[95%] rounded-lg rounded-bl-sm border px-3 py-2 text-sm whitespace-pre-wrap">
                  {revealed || t("ai_settings.test.no_text")}
                </div>
              )}
            </div>

            {result && (
              <div className="space-y-3">
                {result.citations.length > 0 && (
                  <div className="space-y-2">
                    <p className="flex items-center gap-1.5 text-xs font-medium">
                      <BookOpen className="size-3.5" />
                      {t("ai_settings.test.citations")}
                    </p>
                    <ul className="space-y-1.5">
                      {result.citations.map((c) => (
                        <li
                          key={c.chunk_uuid}
                          className="flex items-center justify-between gap-2 rounded-md border px-2 py-1.5 text-xs"
                        >
                          <span className="min-w-0 truncate">
                            {c.title || c.slug}
                          </span>
                          <Badge variant="outline" className="shrink-0">
                            {c.score.toFixed(2)}
                          </Badge>
                        </li>
                      ))}
                    </ul>
                  </div>
                )}
                <dl className="grid grid-cols-2 gap-2">
                  <Stat
                    label={t("ai_settings.runs.tokens")}
                    value={`${formatNumber(result.input_tokens, uiLocale)} / ${formatNumber(result.output_tokens, uiLocale)}`}
                  />
                  <Stat
                    label={t("ai_settings.runs.cost")}
                    value={formatMicros(result.cost_micros, uiLocale)}
                  />
                  <Stat
                    label={t("ai_settings.runs.latency")}
                    value={
                      detail.data
                        ? formatLatency(detail.data.latency_ms, uiLocale)
                        : elapsed !== null
                          ? formatLatency(elapsed, uiLocale)
                          : "-"
                    }
                  />
                  <Stat
                    label={t("ai_settings.runs.model")}
                    value={result.model || "-"}
                  />
                </dl>
                <Button asChild variant="link" size="sm" className="h-auto p-0">
                  <Link
                    href={`${routes.cms.settings.ai}?tab=runs&run=${result.uuid}`}
                  >
                    <ExternalLink className="size-3.5" />
                    {t("ai_settings.test.open_run")}
                  </Link>
                </Button>
              </div>
            )}

            <form
              className="space-y-2"
              onSubmit={(event) => {
                event.preventDefault();
                send();
              }}
            >
              <Textarea
                rows={3}
                maxLength={4000}
                value={message}
                aria-label={t("ai_settings.test.input")}
                placeholder={t("ai_settings.test.placeholder")}
                onChange={(event) => setMessage(event.target.value)}
                onKeyDown={(event) => {
                  if (event.key === "Enter" && !event.shiftKey) {
                    event.preventDefault();
                    send();
                  }
                }}
              />
              <div className="flex items-center justify-between gap-2">
                <span className="text-muted-foreground text-xs">
                  {t("ai_settings.test.cost_note")}
                </span>
                <Button
                  type="submit"
                  size="sm"
                  disabled={!message.trim() || run.isPending}
                >
                  <Send className="size-4" />
                  {t("ai_settings.test.send")}
                </Button>
              </div>
            </form>
          </>
        )}
      </CardContent>
    </Card>
  );
}
