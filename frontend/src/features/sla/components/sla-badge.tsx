"use client";

import { Timer, TimerOff } from "lucide-react";
import { useEffect, useState } from "react";

import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { formatDuration } from "../lib/format";

/** Re-renders every interval so countdowns stay fresh. */
export function useNow(intervalMs = 30_000) {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const id = window.setInterval(() => setNow(Date.now()), intervalMs);
    return () => window.clearInterval(id);
  }, [intervalMs]);
  return now;
}

type SlaFields = {
  first_response_due_at?: string | null;
  next_response_due_at?: string | null;
  resolution_due_at?: string | null;
  sla_breached_at?: string | null;
  status?: string;
};

/** Minutes before the deadline at which the badge turns amber. */
const SOON_MS = 30 * 60_000;

/**
 * Compact SLA timer for the inbox list (CF-46): the nearest open deadline of
 * the conversation, amber when it is close, red once it passed. Paused
 * clocks (pending / snoozed) have no deadline and show nothing.
 */
export function SlaBadge({
  conversation: c,
  className,
}: {
  conversation: SlaFields;
  className?: string;
}) {
  const { t, locale } = useLocale();
  const now = useNow();
  if (c.status === "resolved" || c.status === "closed") return null;
  const dues = [
    { key: "first_response", at: c.first_response_due_at },
    { key: "next_response", at: c.next_response_due_at },
    { key: "resolution", at: c.resolution_due_at },
  ]
    .filter((d): d is { key: string; at: string } => Boolean(d.at))
    .map((d) => ({ ...d, ms: new Date(d.at).getTime() - now }))
    .sort((a, b) => a.ms - b.ms);
  const next = dues[0];
  if (!next) return null;
  const overdue = next.ms <= 0;
  const soon = !overdue && next.ms <= SOON_MS;
  const label = t(`sla.metric.${next.key}`);
  const time = formatDuration(next.ms / 1000, locale);
  return (
    <span
      title={`${label}: ${overdue ? t("sla.badge.overdue", { time }) : t("sla.badge.left", { time })}`}
      className={cn(
        "inline-flex shrink-0 items-center gap-0.5 rounded px-1 text-[10px] font-medium tabular-nums",
        overdue
          ? "bg-red-500/10 text-red-600 dark:text-red-400"
          : soon
            ? "bg-amber-500/10 text-amber-700 dark:text-amber-400"
            : "text-muted-foreground",
        className,
      )}
    >
      {overdue ? (
        <TimerOff className="size-3" aria-hidden />
      ) : (
        <Timer className="size-3" aria-hidden />
      )}
      {overdue ? `-${time}` : time}
    </span>
  );
}
