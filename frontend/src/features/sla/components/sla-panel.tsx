"use client";

import { useQuery } from "@tanstack/react-query";
import { CircleCheck, CirclePause, Timer, TimerOff } from "lucide-react";

import { Permission } from "@/config/permissions";
import { useActiveOrganization } from "@/features/organizations";
import { cn, datetime } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { formatDuration } from "../lib/format";
import { slaService, type SlaMetric } from "../services/sla.service";
import { useNow } from "./sla-badge";

/**
 * SLA clocks of one conversation for the inbox context pane (CF-46). The
 * clocks follow the conversation through an event consumer a moment after
 * each change, so the query refreshes on the fields that move them and
 * every 30 seconds.
 */
export function SlaPanel({
  conversation: c,
}: {
  conversation: {
    uuid: string;
    status: string;
    priority: string;
    first_response_at?: string | null;
    waiting_since?: string | null;
  };
}) {
  const { t, locale } = useLocale();
  const { active: org } = useActiveOrganization();
  const enabled = (org?.permissions ?? []).includes(
    Permission.TenantConversationsRead,
  );
  const now = useNow();
  const { data } = useQuery({
    queryKey: [
      "tenant",
      org?.slug,
      "conversation-sla",
      c.uuid,
      c.status,
      c.priority,
      c.first_response_at ?? "",
      c.waiting_since ?? "",
    ],
    queryFn: () => slaService.conversation(c.uuid),
    enabled: Boolean(org) && enabled,
    refetchInterval: 30_000,
  });
  if (!data || data.metrics.length === 0) return null;
  return (
    <section>
      <h3 className="text-muted-foreground mb-2 flex items-center justify-between text-[11px] font-semibold tracking-wide uppercase">
        <span>{t("sla.panel.title")}</span>
        {data.policy && (
          <span className="truncate font-normal normal-case">
            {data.policy.name}
          </span>
        )}
      </h3>
      <ul className="space-y-1.5">
        {data.metrics.map((m) => (
          <MetricRow
            key={m.metric}
            metric={m}
            now={now}
            locale={locale}
            t={t}
          />
        ))}
      </ul>
    </section>
  );
}

function MetricRow({
  metric: m,
  now,
  locale,
  t,
}: {
  metric: SlaMetric;
  now: number;
  locale: string;
  t: (key: string, params?: Record<string, string | number>) => string;
}) {
  let detail: string;
  let tone = "text-muted-foreground";
  let Icon = Timer;
  switch (m.status) {
    case "met":
      Icon = CircleCheck;
      tone = "text-emerald-600 dark:text-emerald-400";
      detail = m.completed_at ? datetime(m.completed_at, "dd.MM HH:mm") : "";
      break;
    case "breached":
      Icon = TimerOff;
      tone = "text-red-600 dark:text-red-400";
      detail = m.breached_at
        ? t("sla.panel.breached_at", {
            time: datetime(m.breached_at, "dd.MM HH:mm"),
          })
        : "";
      break;
    case "paused":
      Icon = CirclePause;
      detail = t("sla.panel.paused", {
        time: formatDuration(m.remaining_seconds, locale),
      });
      break;
    default: {
      const left = m.due_at
        ? (new Date(m.due_at).getTime() - now) / 1000
        : m.remaining_seconds;
      if (left <= 0) {
        Icon = TimerOff;
        tone = "text-red-600 dark:text-red-400";
      } else if (m.warn_at && new Date(m.warn_at).getTime() <= now) {
        tone = "text-amber-700 dark:text-amber-400";
      }
      detail =
        left <= 0
          ? t("sla.badge.overdue", { time: formatDuration(left, locale) })
          : t("sla.badge.left", { time: formatDuration(left, locale) });
    }
  }
  return (
    <li className="flex items-center gap-2 text-xs">
      <Icon className={cn("size-3.5 shrink-0", tone)} aria-hidden />
      <span className="flex-1 truncate">
        {t(`sla.metric.${m.metric}`)}
        {m.metric === "next_response" && m.cycle > 1 ? ` #${m.cycle}` : ""}
      </span>
      <span className={cn("shrink-0 tabular-nums", tone)}>
        {t(`sla.status.${m.status}`)}
        {detail ? ` · ${detail}` : ""}
      </span>
    </li>
  );
}
