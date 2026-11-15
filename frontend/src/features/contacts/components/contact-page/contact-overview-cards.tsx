"use client";

import Link from "next/link";
import { useState } from "react";
import {
  AlertTriangle,
  CalendarClock,
  CircleDot,
  Clock,
  FileText,
  Image as ImageIcon,
  Megaphone,
  MessagesSquare,
  RefreshCw,
  ShieldAlert,
  ShieldCheck,
  Sparkles,
  Star,
  UserRound,
} from "lucide-react";

import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { routes } from "@/config/routes";
import { aiErrorMessage } from "@/features/ai-settings/lib/errors";
import { ChannelGlyph } from "@/features/inbox/components/channel-glyph";
import { shortAgo } from "@/features/inbox/lib/format";
import { cn, fileSize } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { useGenerateContactSummary } from "../../hooks/use-contact-profile";
import type { ContactOverview } from "../../services/profile.service";
import { dateOf, dateTimeOf } from "./lib";

/** Health and status summary of the contact (CF-221). */
export function ContactStats({ data }: { data: ContactOverview }) {
  const { t, locale } = useLocale();
  const { stats, sections, appointments } = data;
  // Fixed at mount: the page refetches when it matters.
  const [now] = useState(() => Date.now());
  const next = appointments.find(
    (a) =>
      new Date(a.starts_at).getTime() > now &&
      a.status !== "cancelled" &&
      a.status !== "no_show",
  );
  const total = stats.open_conversations + stats.closed_conversations;
  const sla =
    stats.sla_breached > 0
      ? {
          tone: "danger" as const,
          icon: ShieldAlert,
          value: t("contacts.profile.stats.sla_breached", {
            count: stats.sla_breached,
          }),
        }
      : stats.sla_at_risk > 0
        ? {
            tone: "warning" as const,
            icon: AlertTriangle,
            value: t("contacts.profile.stats.sla_at_risk", {
              count: stats.sla_at_risk,
            }),
          }
        : {
            tone: "success" as const,
            icon: ShieldCheck,
            value:
              stats.open_conversations > 0
                ? t("contacts.profile.stats.sla_ok")
                : "—",
          };
  return (
    <section
      aria-label={t("contacts.profile.stats.label")}
      className="grid grid-cols-2 gap-3 sm:grid-cols-3 xl:grid-cols-6"
    >
      {sections.conversations && (
        <>
          <Stat
            icon={MessagesSquare}
            label={t("contacts.profile.stats.open")}
            value={String(stats.open_conversations)}
            hint={t("contacts.profile.stats.total", { count: total })}
            accent={stats.open_conversations > 0}
          />
          <Stat
            icon={sla.icon}
            label={t("contacts.profile.stats.sla")}
            value={sla.value}
            tone={stats.open_conversations > 0 ? sla.tone : undefined}
          />
          <Stat
            icon={Clock}
            label={t("contacts.profile.stats.last_contact")}
            value={
              stats.last_conversation_at
                ? shortAgo(stats.last_conversation_at, locale)
                : "—"
            }
            hint={
              stats.last_inbound_at
                ? t("contacts.profile.stats.last_inbound", {
                    ago: shortAgo(stats.last_inbound_at, locale),
                  })
                : undefined
            }
          />
          <Stat
            icon={Star}
            label={t("contacts.profile.stats.csat")}
            value={
              stats.csat_average != null
                ? `${stats.csat_average.toFixed(1)} / 5`
                : "—"
            }
            hint={
              stats.nps_latest != null
                ? t("contacts.profile.stats.nps", { score: stats.nps_latest })
                : t("contacts.account.csat_hint", { count: stats.csat_count })
            }
            tone={
              stats.csat_average == null
                ? undefined
                : stats.csat_average >= 4
                  ? "success"
                  : stats.csat_average < 3
                    ? "danger"
                    : "warning"
            }
          />
        </>
      )}
      <Stat
        icon={CalendarClock}
        label={t("contacts.profile.stats.next_appointment")}
        value={next ? dateOf(next.starts_at, locale) : "—"}
        hint={next ? next.title || next.type : undefined}
      />
      <Stat
        icon={UserRound}
        label={t("contacts.profile.stats.customer_since")}
        value={dateOf(
          stats.first_conversation_at ?? data.contact.created_at,
          locale,
        )}
        hint={t("contacts.profile.stats.notes", { count: stats.note_count })}
      />
    </section>
  );
}

function Stat({
  icon: Icon,
  label,
  value,
  hint,
  tone,
  accent,
}: {
  icon: typeof Star;
  label: string;
  value: string;
  hint?: string;
  tone?: "success" | "warning" | "danger";
  accent?: boolean;
}) {
  return (
    <div
      className={cn(
        "bg-card flex min-w-0 flex-col gap-1 rounded-xl border p-3",
        accent && "border-signal/40",
      )}
    >
      <p className="text-muted-foreground flex items-center gap-1.5 text-xs">
        <Icon
          aria-hidden
          className={cn(
            "size-3.5 shrink-0",
            tone === "success" && "text-success",
            tone === "warning" && "text-warning",
            tone === "danger" && "text-destructive",
          )}
        />
        <span className="truncate">{label}</span>
      </p>
      <p
        className={cn(
          "truncate text-lg font-semibold tabular-nums",
          tone === "danger" && "text-destructive",
        )}
      >
        {value}
      </p>
      {hint && <p className="text-muted-foreground truncate text-xs">{hint}</p>}
    </div>
  );
}

const SENTIMENT_VARIANT = {
  positive: "success",
  neutral: "secondary",
  negative: "danger",
  unknown: "outline",
} as const;

/** Cached AI summary with open topics; generated on demand (CF-221). */
export function ContactAISummaryCard({ data }: { data: ContactOverview }) {
  const { t, locale } = useLocale();
  const generate = useGenerateContactSummary(data.contact.uuid);
  const summary = data.ai_summary;
  if (!data.sections.ai) return null;
  const run = () => generate.mutate(locale === "en" ? "en" : "tr");
  return (
    <section
      aria-labelledby="contact-ai-summary"
      aria-busy={generate.isPending}
      className="bg-card relative overflow-hidden rounded-2xl border p-4"
    >
      <div
        aria-hidden
        className="bg-signal/10 pointer-events-none absolute -top-10 -right-10 size-32 rounded-full blur-2xl"
      />
      <div className="relative mb-3 flex items-center gap-2">
        <span className="bg-signal-soft text-signal-text flex size-7 items-center justify-center rounded-lg">
          <Sparkles className="size-4" />
        </span>
        <h2 id="contact-ai-summary" className="font-medium">
          {t("contacts.profile.ai.title")}
        </h2>
        {summary && (
          <Badge
            variant={SENTIMENT_VARIANT[summary.sentiment]}
            className="ml-auto"
          >
            {t(`contacts.profile.ai.sentiment.${summary.sentiment}`)}
          </Badge>
        )}
      </div>
      {generate.isPending ? (
        <div
          className="grid gap-2"
          aria-label={t("contacts.profile.ai.generating")}
        >
          <Skeleton className="h-3.5 w-full" />
          <Skeleton className="h-3.5 w-11/12" />
          <Skeleton className="h-3.5 w-3/4" />
          <Skeleton className="mt-2 h-3.5 w-1/2" />
        </div>
      ) : summary ? (
        <div className="relative grid gap-3">
          <p className="text-sm leading-relaxed">{summary.summary}</p>
          {summary.open_topics.length > 0 && (
            <div>
              <h3 className="text-muted-foreground mb-1.5 text-[11px] font-semibold tracking-wide uppercase">
                {t("contacts.profile.ai.open_topics")}
              </h3>
              <ul className="grid gap-1">
                {summary.open_topics.map((topic) => (
                  <li key={topic} className="flex gap-2 text-sm">
                    <CircleDot className="text-signal-text mt-0.5 size-3.5 shrink-0" />
                    <span>{topic}</span>
                  </li>
                ))}
              </ul>
            </div>
          )}
          {summary.stale && (
            <p className="text-warning flex items-center gap-1.5 text-xs">
              <AlertTriangle className="size-3.5" />
              {t("contacts.profile.ai.stale")}
            </p>
          )}
        </div>
      ) : (
        <p className="text-muted-foreground relative text-sm">
          {t("contacts.profile.ai.empty")}
        </p>
      )}
      {generate.isError && (
        <Alert variant="destructive" className="mt-3">
          <AlertDescription>
            {aiErrorMessage(generate.error, t)}
          </AlertDescription>
        </Alert>
      )}
      <div className="relative mt-3 flex items-center gap-2 border-t pt-3">
        <p className="text-muted-foreground min-w-0 flex-1 truncate text-xs">
          {summary
            ? t("contacts.profile.ai.generated", {
                when: dateTimeOf(summary.generated_at, locale),
              })
            : t("contacts.profile.ai.disclaimer")}
        </p>
        <Button
          size="sm"
          variant={summary && !summary.stale ? "ghost" : "default"}
          disabled={generate.isPending}
          onClick={run}
        >
          {summary ? (
            <RefreshCw
              className={cn("size-4", generate.isPending && "animate-spin")}
            />
          ) : (
            <Sparkles className="size-4" />
          )}
          {summary
            ? t("contacts.profile.ai.refresh")
            : t("contacts.profile.ai.generate")}
        </Button>
      </div>
    </section>
  );
}

/** Channel identities: how the contact is known on each channel. */
export function IdentitiesCard({ data }: { data: ContactOverview }) {
  const { t, locale } = useLocale();
  const ids = data.contact.identities ?? [];
  return (
    <section className="bg-card rounded-2xl border p-4">
      <h2 className="mb-2 font-medium">{t("contacts.identities")}</h2>
      {ids.length === 0 ? (
        <p className="text-muted-foreground text-sm">
          {t("contacts.no_identities")}
        </p>
      ) : (
        <ul className="grid gap-1.5">
          {ids.map((i) => (
            <li
              key={i.id}
              className="bg-muted/40 flex min-w-0 items-center gap-2 rounded-lg px-2.5 py-2 text-xs"
            >
              <ChannelGlyph type={i.channel_type} className="size-4 shrink-0" />
              <div className="min-w-0 flex-1">
                <p className="font-medium">
                  {t(`channels.types.${i.channel_type}`)}
                </p>
                <p className="text-muted-foreground truncate">
                  {i.username || i.display_name || i.external_id}
                </p>
              </div>
              {i.last_seen_at && (
                <span className="text-muted-foreground shrink-0 tabular-nums">
                  {shortAgo(i.last_seen_at, locale)}
                </span>
              )}
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

/** Files of the contact's conversations; each opens in its conversation. */
export function FilesCard({ data }: { data: ContactOverview }) {
  const { t, locale } = useLocale();
  if (!data.sections.conversations) return null;
  const files = data.attachments;
  return (
    <section className="bg-card rounded-2xl border p-4">
      <div className="mb-2 flex items-baseline gap-2">
        <h2 className="font-medium">{t("contacts.profile.files")}</h2>
        <span className="text-muted-foreground text-xs tabular-nums">
          {data.stats.attachment_count}
        </span>
      </div>
      {files.length === 0 ? (
        <p className="text-muted-foreground text-sm">
          {t("contacts.profile.files_empty")}
        </p>
      ) : (
        <ul className="grid gap-1">
          {files.slice(0, 8).map((f) => {
            const Icon = f.content_type.startsWith("image/")
              ? ImageIcon
              : FileText;
            return (
              <li key={f.uuid}>
                <Link
                  href={`${routes.cms.inbox}?c=${f.conversation_uuid}`}
                  className="hover:bg-muted focus-visible:ring-ring flex min-w-0 items-center gap-2 rounded-md px-2 py-1.5 text-sm outline-none focus-visible:ring-2"
                >
                  <Icon className="text-muted-foreground size-4 shrink-0" />
                  <span className="min-w-0 flex-1 truncate">{f.filename}</span>
                  <span className="text-muted-foreground shrink-0 text-xs tabular-nums">
                    {fileSize(f.size_bytes)} · #{f.conversation_number}
                  </span>
                </Link>
              </li>
            );
          })}
        </ul>
      )}
      {files.length > 0 && (
        <p className="text-muted-foreground mt-2 text-[11px]">
          {t("contacts.profile.files_hint", {
            date: dateOf(files[0].created_at, locale),
          })}
        </p>
      )}
    </section>
  );
}

/** Status incidents the contact was notified about. */
export function StatusNoticesCard({ data }: { data: ContactOverview }) {
  const { t, locale } = useLocale();
  if (!data.sections.conversations || data.status_notices.length === 0)
    return null;
  return (
    <section className="bg-card rounded-2xl border p-4">
      <h2 className="mb-2 font-medium">
        {t("contacts.profile.status_notices")}
      </h2>
      <ul className="grid gap-1.5">
        {data.status_notices.slice(0, 5).map((n) => (
          <li key={n.uuid}>
            <Link
              href={routes.cms.incidents}
              className="hover:bg-muted focus-visible:ring-ring flex min-w-0 items-center gap-2 rounded-md px-2 py-1.5 text-sm outline-none focus-visible:ring-2"
            >
              <Megaphone className="text-destructive size-4 shrink-0" />
              <span className="min-w-0 flex-1 truncate">{n.title}</span>
              <span className="text-muted-foreground shrink-0 text-xs">
                {dateOf(n.created_at, locale)}
              </span>
            </Link>
          </li>
        ))}
      </ul>
    </section>
  );
}
