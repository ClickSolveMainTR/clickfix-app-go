"use client";

import {
  ArrowDownToLine,
  BookOpen,
  ChevronLeft,
  ChevronRight,
  Copy,
  CornerDownLeft,
  Lock,
  RefreshCw,
  Replace,
  Sparkles,
  X,
} from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import type { Copilot, CopilotEntry } from "../hooks/use-copilot";
import {
  copilotErrorKey,
  copilotKind,
  stripMarkers,
  useTypewriter,
} from "../lib/copilot";
import type { AICitation } from "../services/copilot.service";

export type CopilotApply = "replace" | "append" | "note";

type Props = {
  copilot: Copilot;
  /** Agent's current draft is empty (insert instead of replace). */
  draftEmpty: boolean;
  canNote: boolean;
  canReply: boolean;
  onApply: (text: string, how: CopilotApply) => void;
  onRetry: (entry: CopilotEntry) => void;
  onAsk: (question: string) => void;
  /** Agent asked to type a question (Q / menu); focuses the ask field. */
  asking: boolean;
  onAskingChange: (asking: boolean) => void;
};

/**
 * Copilot results docked above the composer (CF-66). One answer at a time
 * with a short history; drafts go into the reply, insights can be kept as an
 * internal note. Sources are the knowledge chunks the answer was grounded on.
 */
export function CopilotPanel({
  copilot,
  draftEmpty,
  canNote,
  canReply,
  onApply,
  onRetry,
  onAsk,
  asking,
  onAskingChange,
}: Props) {
  const { t } = useLocale();
  const entry = copilot.current;

  // Esc inside the panel closes it (or cancels the request).
  const onKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Escape") {
      e.preventDefault();
      e.stopPropagation();
      if (asking) onAskingChange(false);
      else copilot.close();
    }
  };

  if (!asking && !entry) return null;

  return (
    <div
      role="region"
      aria-label={t("inbox.copilot.title")}
      onKeyDown={onKeyDown}
      className="bg-card animate-in fade-in slide-in-from-bottom-1 relative mb-2 overflow-hidden rounded-xl border shadow-sm duration-200"
    >
      <span
        aria-hidden
        className={cn(
          "bg-signal absolute inset-x-0 top-0 h-0.5 origin-left",
          entry?.status === "pending"
            ? "animate-pulse opacity-100"
            : "opacity-60",
        )}
      />
      <header className="flex items-center gap-2 px-3 pt-2.5 pb-1.5">
        <span className="bg-signal-soft text-signal-text flex size-6 shrink-0 items-center justify-center rounded-full">
          <Sparkles className="size-3.5" />
        </span>
        <p className="min-w-0 flex-1 truncate text-xs font-semibold">
          {asking && !entry
            ? t("inbox.copilot.op.ask")
            : entry
              ? entryTitle(entry, t)
              : null}
        </p>
        {copilot.entries.length > 1 && !asking && (
          <div className="text-muted-foreground flex items-center text-[11px] tabular-nums">
            <Button
              variant="ghost"
              size="icon"
              className="size-6"
              aria-label={t("inbox.copilot.newer")}
              disabled={copilot.index === 0}
              onClick={() => copilot.setIndex(copilot.index - 1)}
            >
              <ChevronLeft className="size-3.5" />
            </Button>
            {copilot.index + 1}/{copilot.entries.length}
            <Button
              variant="ghost"
              size="icon"
              className="size-6"
              aria-label={t("inbox.copilot.older")}
              disabled={copilot.index >= copilot.entries.length - 1}
              onClick={() => copilot.setIndex(copilot.index + 1)}
            >
              <ChevronRight className="size-3.5" />
            </Button>
          </div>
        )}
        <Button
          variant="ghost"
          size="icon"
          className="size-6"
          aria-label={t("inbox.copilot.close")}
          onClick={() => {
            onAskingChange(false);
            copilot.close();
          }}
        >
          <X className="size-3.5" />
        </Button>
      </header>

      {asking && (
        <AskField
          onSubmit={(q) => {
            onAskingChange(false);
            onAsk(q);
          }}
          onCancel={() => onAskingChange(false)}
        />
      )}

      {!asking && entry && (
        <div className="max-h-[min(45vh,22rem)] overflow-y-auto px-3 pb-3">
          {entry.status === "pending" && (
            <Pending entry={entry} onCancel={copilot.cancel} />
          )}
          {entry.status === "error" && (
            <div className="space-y-2 py-1">
              <p className="text-destructive text-sm">
                {t(copilotErrorKey(entry.error))}
              </p>
              <Button
                size="sm"
                variant="outline"
                onClick={() => onRetry(entry)}
              >
                <RefreshCw className="size-3.5" />
                {t("inbox.copilot.retry")}
              </Button>
            </div>
          )}
          {entry.status === "done" && entry.text != null && (
            <Result
              entry={entry}
              draftEmpty={draftEmpty}
              canNote={canNote}
              canReply={canReply}
              onApply={onApply}
              onRetry={() => onRetry(entry)}
            />
          )}
        </div>
      )}
    </div>
  );
}

function entryTitle(
  entry: CopilotEntry,
  t: (key: string, vars?: Record<string, string | number>) => string,
) {
  if (entry.origin === "agent") return t("inbox.copilot.agent_draft");
  const base = t(`inbox.copilot.op.${entry.op}`);
  if (entry.op === "tone" && entry.option)
    return `${base} · ${t(`inbox.copilot.tone.${entry.option}`)}`;
  if (entry.op === "translate" && entry.option)
    return `${base} · ${t(`inbox.copilot.language.${entry.option}`)}`;
  return base;
}

function AskField({
  onSubmit,
  onCancel,
}: {
  onSubmit: (q: string) => void;
  onCancel: () => void;
}) {
  const { t } = useLocale();
  const [q, setQ] = useState("");
  const input = useRef<HTMLInputElement>(null);
  useEffect(() => input.current?.focus(), []);
  return (
    <form
      className="flex items-center gap-2 px-3 pb-3"
      onSubmit={(e) => {
        e.preventDefault();
        if (q.trim()) onSubmit(q.trim());
      }}
    >
      <input
        ref={input}
        value={q}
        maxLength={1000}
        onChange={(e) => setQ(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Escape") {
            e.preventDefault();
            e.stopPropagation();
            onCancel();
          }
        }}
        aria-label={t("inbox.copilot.ask_label")}
        placeholder={t("inbox.copilot.ask_placeholder")}
        className="bg-background focus-visible:ring-ring/40 h-9 min-w-0 flex-1 rounded-lg border px-3 text-sm outline-none focus-visible:ring-2"
      />
      <Button type="submit" size="sm" disabled={!q.trim()}>
        <CornerDownLeft className="size-3.5" />
        <span className="sr-only sm:not-sr-only">
          {t("inbox.copilot.ask_submit")}
        </span>
      </Button>
    </form>
  );
}

function Pending({
  entry,
  onCancel,
}: {
  entry: CopilotEntry;
  onCancel: () => void;
}) {
  const { t } = useLocale();
  const [elapsed, setElapsed] = useState(0);
  useEffect(() => {
    const id = setInterval(
      () =>
        setElapsed(Math.floor((performance.now() - entry.startedAt) / 1000)),
      500,
    );
    return () => clearInterval(id);
  }, [entry.startedAt]);
  return (
    <div className="space-y-2 py-1" aria-live="polite" aria-busy>
      <div className="space-y-1.5" aria-hidden>
        {[92, 78, 54].map((w, i) => (
          <div
            key={w}
            className="bg-muted h-3 animate-pulse rounded"
            style={{ width: `${w}%`, animationDelay: `${i * 120}ms` }}
          />
        ))}
      </div>
      <div className="text-muted-foreground flex items-center gap-2 text-xs">
        <span className="bg-signal size-1.5 animate-pulse rounded-full" />
        <span>{t("inbox.copilot.writing")}</span>
        <span className="tabular-nums">
          {elapsed > 0 ? t("inbox.copilot.seconds", { n: elapsed }) : ""}
        </span>
        <Button
          variant="ghost"
          size="sm"
          className="ml-auto h-7 text-xs"
          onClick={onCancel}
        >
          {t("inbox.copilot.cancel")}
          <kbd className="text-muted-foreground/70 font-mono text-[10px]">
            Esc
          </kbd>
        </Button>
      </div>
    </div>
  );
}

function Result({
  entry,
  draftEmpty,
  canNote,
  canReply,
  onApply,
  onRetry,
}: {
  entry: CopilotEntry;
  draftEmpty: boolean;
  canNote: boolean;
  canReply: boolean;
  onApply: (text: string, how: CopilotApply) => void;
  onRetry: () => void;
}) {
  const { t } = useLocale();
  const full = entry.text ?? "";
  const shown = useTypewriter(full, entry.id);
  const typing = shown.length < full.length;
  const kind = copilotKind(entry.op);
  const [showOriginal, setShowOriginal] = useState(false);
  const primary = useRef<HTMLButtonElement>(null);
  const sources = entry.sources ?? [];

  // Keyboard flow: the main action is focused once the text settles, so
  // Enter applies it and Esc dismisses the panel.
  useEffect(() => {
    if (!typing) primary.current?.focus({ preventScroll: true });
  }, [typing]);

  const clean = stripMarkers(full);
  const copy = () =>
    navigator.clipboard
      .writeText(clean)
      .then(() => toast.success(t("inbox.copilot.copied")))
      .catch(() => toast.error(t("inbox.copilot.copy_failed")));

  return (
    <div className="space-y-3">
      <p
        className="text-sm leading-relaxed break-words whitespace-pre-wrap"
        aria-live="polite"
      >
        <MarkedText text={shown} sources={sources} />
        {typing && (
          <span
            aria-hidden
            className="bg-signal ml-0.5 inline-block h-3.5 w-[3px] translate-y-0.5 animate-pulse rounded-sm"
          />
        )}
      </p>

      {entry.input && entry.op !== "ask" && (
        <div>
          <button
            type="button"
            className="text-muted-foreground hover:text-foreground text-[11px] underline-offset-2 hover:underline"
            aria-expanded={showOriginal}
            onClick={() => setShowOriginal((o) => !o)}
          >
            {showOriginal
              ? t("inbox.copilot.hide_original")
              : t("inbox.copilot.show_original")}
          </button>
          {showOriginal && (
            <p className="bg-muted/60 text-muted-foreground mt-1 rounded-md px-2.5 py-2 text-xs whitespace-pre-wrap">
              {entry.input}
            </p>
          )}
        </div>
      )}
      {entry.op === "ask" && entry.input && (
        <p className="text-muted-foreground text-[11px]">
          {t("inbox.copilot.asked", { question: entry.input })}
        </p>
      )}

      {sources.length > 0 && <Sources sources={sources} />}
      {entry.op === "suggest_reply" &&
        entry.origin === "copilot" &&
        sources.length === 0 && (
          <p className="text-muted-foreground flex items-center gap-1.5 text-[11px]">
            <BookOpen className="size-3" />
            {t("inbox.copilot.no_sources")}
          </p>
        )}

      <div className="flex flex-wrap items-center gap-1.5">
        {kind === "draft" && canReply && (
          <>
            <Button
              ref={primary}
              size="sm"
              disabled={typing}
              onClick={() => onApply(clean, draftEmpty ? "append" : "replace")}
            >
              {draftEmpty ? (
                <ArrowDownToLine className="size-3.5" />
              ) : (
                <Replace className="size-3.5" />
              )}
              {draftEmpty
                ? t("inbox.copilot.insert")
                : t("inbox.copilot.replace")}
            </Button>
            {!draftEmpty && entry.op === "suggest_reply" && (
              <Button
                size="sm"
                variant="outline"
                disabled={typing}
                onClick={() => onApply(clean, "append")}
              >
                {t("inbox.copilot.append")}
              </Button>
            )}
          </>
        )}
        {kind === "insight" && canNote && (
          <Button
            ref={primary}
            size="sm"
            variant="outline"
            disabled={typing}
            onClick={() => onApply(clean, "note")}
          >
            <Lock className="size-3.5" />
            {t("inbox.copilot.as_note")}
          </Button>
        )}
        <Button
          size="sm"
          variant="ghost"
          disabled={typing}
          onClick={() => void copy()}
        >
          <Copy className="size-3.5" />
          {t("inbox.copilot.copy")}
        </Button>
        {entry.origin === "copilot" && (
          <Button size="sm" variant="ghost" onClick={onRetry}>
            <RefreshCw className="size-3.5" />
            {t("inbox.copilot.regenerate")}
          </Button>
        )}
        <Meta entry={entry} />
      </div>
      <p className="text-muted-foreground text-[11px]">
        {t("inbox.copilot.disclaimer")}
      </p>
    </div>
  );
}

/** Renders `[n]` markers as source pills. */
function MarkedText({
  text,
  sources,
}: {
  text: string;
  sources: AICitation[];
}) {
  if (sources.length === 0) return <>{text}</>;
  const parts = text.split(/(\[\d+\])/g);
  return (
    <>
      {parts.map((part, i) => {
        const m = /^\[(\d+)\]$/.exec(part);
        if (!m) return <span key={i}>{part}</span>;
        return (
          <sup
            key={i}
            className="bg-signal-soft text-signal-text mx-0.5 rounded px-1 py-px text-[10px] font-semibold"
          >
            {m[1]}
          </sup>
        );
      })}
    </>
  );
}

function Sources({ sources }: { sources: AICitation[] }) {
  const { t } = useLocale();
  return (
    <div className="space-y-1">
      <p className="text-muted-foreground text-[11px] font-medium tracking-wide uppercase">
        {t("inbox.copilot.sources")}
      </p>
      <ul className="flex flex-wrap gap-1.5">
        {sources.map((s, i) => (
          <li key={s.chunk_uuid}>
            <Popover>
              <PopoverTrigger asChild>
                <button
                  type="button"
                  className="hover:bg-muted focus-visible:ring-ring/50 flex max-w-60 items-center gap-1.5 rounded-md border px-2 py-1 text-left text-xs outline-none focus-visible:ring-2"
                >
                  <span className="bg-signal-soft text-signal-text rounded px-1 text-[10px] font-semibold tabular-nums">
                    {i + 1}
                  </span>
                  <span className="truncate">{s.title}</span>
                </button>
              </PopoverTrigger>
              <PopoverContent className="w-80 space-y-2" align="start">
                <p className="text-sm font-medium">{s.title}</p>
                <p className="text-muted-foreground line-clamp-6 text-xs whitespace-pre-wrap">
                  {s.content}
                </p>
                <p className="text-muted-foreground flex flex-wrap gap-x-3 text-[11px]">
                  <span>/{s.slug}</span>
                  <span>{s.locale.toUpperCase()}</span>
                  <span>{t("inbox.copilot.version", { n: s.version })}</span>
                  <span>
                    {t("inbox.copilot.score", {
                      n: Math.round(s.score * 100),
                    })}
                  </span>
                </p>
              </PopoverContent>
            </Popover>
          </li>
        ))}
      </ul>
    </div>
  );
}

function Meta({ entry }: { entry: CopilotEntry }) {
  const { t } = useLocale();
  if (entry.origin === "agent") return null;
  const bits = [
    entry.latencyMs != null
      ? t("inbox.copilot.seconds", {
          n: (entry.latencyMs / 1000).toFixed(1),
        })
      : null,
    entry.tokens ? t("inbox.copilot.tokens", { n: entry.tokens }) : null,
    entry.credits ? t("inbox.copilot.credits", { n: entry.credits }) : null,
  ].filter(Boolean);
  if (bits.length === 0) return null;
  return (
    <span
      className="text-muted-foreground ml-auto text-[11px] tabular-nums"
      title={entry.model}
    >
      {bits.join(" · ")}
    </span>
  );
}
