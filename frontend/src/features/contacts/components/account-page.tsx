"use client";

import { useState } from "react";
import Link from "next/link";
import {
  ArrowLeft,
  Building2,
  Pencil,
  CheckCircle2,
  HeartPulse,
  MessageSquare,
  StickyNote,
  Star,
  Trash2,
  Wallet,
  Zap,
} from "lucide-react";

import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Textarea } from "@/components/ui/textarea";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { formatMinor } from "@/features/billing/lib/money";
import {
  CustomFieldValue,
  useCustomFieldDefinitions,
} from "@/features/custom-fields";
import { LinkedAppointmentsCard } from "@/features/calendar";
import { ChannelGlyph } from "@/features/inbox/components/channel-glyph";
import { shortAgo } from "@/features/inbox/lib/format";
import { useLocale } from "@/providers/locale-provider";

import {
  useCompanyNoteMutations,
  useCompanyOverview,
  useCompanyTimeline,
} from "../hooks/use-account";
import { useContactList, useContactsScope } from "../hooks/use-contacts";
import type {
  CompanyOverview,
  CompanyTimelineItem,
  CompanyTimelineKind,
} from "../services/contacts.service";
import { CompanyDialog } from "./companies-page";
import { CompanyHealthBadge } from "./company-health-badge";
import { CompanyHealthCard } from "./company-health-card";
import { CompanyRevenueCard } from "./company-revenue-card";

type Filter = "all" | "conversations" | "notes" | "events" | "billing";

const FILTER_KINDS: Record<Filter, CompanyTimelineKind[]> = {
  all: [],
  conversations: ["conversation_opened", "conversation_resolved", "csat"],
  notes: ["note"],
  events: ["event"],
  billing: ["revenue", "health"],
};

/** Account screen of a company with its unified timeline (CF-85). */
export function AccountPage({ uuid }: { uuid: string }) {
  const { t, locale } = useLocale();
  const scope = useContactsScope();
  const overview = useCompanyOverview(uuid);
  const [editing, setEditing] = useState(false);

  if (!scope.canRead) {
    return (
      <p className="text-muted-foreground p-8 text-sm">
        {t("contacts.no_permission")}
      </p>
    );
  }
  if (overview.isLoading) {
    return (
      <div className="mx-auto grid w-full max-w-6xl gap-4">
        <Skeleton className="h-20 rounded-xl" />
        <Skeleton className="h-96 rounded-xl" />
      </div>
    );
  }
  if (!overview.data) {
    return (
      <p className="text-muted-foreground p-8 text-sm">
        {t("contacts.account.not_found")}
      </p>
    );
  }
  const { company, stats } = overview.data;
  const revenue = company.revenue;
  return (
    <div className="mx-auto flex w-full max-w-6xl flex-col gap-4">
      <Link
        href="/companies"
        className="text-muted-foreground hover:text-foreground flex w-fit items-center gap-1 text-sm"
      >
        <ArrowLeft className="size-4" />
        {t("contacts.companies.title")}
      </Link>
      <header className="flex flex-wrap items-center gap-3">
        <span className="bg-muted flex size-11 items-center justify-center rounded-xl">
          <Building2 className="size-5" />
        </span>
        <div className="min-w-0">
          <h1 className="truncate text-xl font-semibold">{company.name}</h1>
          <p className="text-muted-foreground truncate text-sm">
            {[company.domain, company.tier].filter(Boolean).join(" · ") || "—"}
          </p>
        </div>
        <div className="ml-auto flex items-center gap-2">
          <CompanyHealthBadge health={company.health} />
          <span className="text-muted-foreground text-sm">
            {t("contacts.account.owner")}:{" "}
            <span className="text-foreground font-medium">
              {company.account_owner?.name ?? t("contacts.account.no_owner")}
            </span>
          </span>
          {scope.canWrite && (
            <Button
              size="sm"
              variant="outline"
              onClick={() => setEditing(true)}
            >
              <Pencil className="size-4" />
              {t("contacts.account.edit")}
            </Button>
          )}
        </div>
      </header>
      {editing && (
        <CompanyDialog company={company} onClose={() => setEditing(false)} />
      )}

      <div className="grid grid-cols-2 gap-3 md:grid-cols-5">
        <Stat
          label={t("contacts.account.open")}
          value={String(stats.open_conversations)}
        />
        <Stat
          label={t("contacts.account.closed")}
          value={String(stats.closed_conversations)}
        />
        <Stat
          label={t("contacts.account.csat")}
          value={
            stats.csat_average != null
              ? `${stats.csat_average.toFixed(1)} / 5`
              : "—"
          }
          hint={t("contacts.account.csat_hint", { count: stats.csat_count })}
        />
        <Stat
          label={t("contacts.account.mrr")}
          value={
            revenue && revenue.mrr_minor > 0
              ? formatMinor(revenue.mrr_minor, revenue.currency, locale)
              : "—"
          }
          hint={revenue?.plan || undefined}
        />
        <Stat
          label={t("contacts.account.last_contact")}
          value={
            stats.last_conversation_at
              ? shortAgo(stats.last_conversation_at, locale)
              : "—"
          }
        />
      </div>

      <div className="grid gap-4 lg:grid-cols-3">
        <section className="bg-card flex flex-col gap-3 rounded-xl border p-4 lg:col-span-2">
          <Timeline uuid={uuid} />
        </section>
        <aside className="flex flex-col gap-4">
          <OpenConversations overview={overview.data} />
          <CompanyContacts uuid={uuid} />
          <CustomFieldsCard values={company.custom_fields} />
          <CompanyHealthCard companyUuid={uuid} />
          <CompanyRevenueCard companyUuid={uuid} />
          <LinkedAppointmentsCard
            context={{ companyUuid: uuid, label: company.name }}
          />
        </aside>
      </div>
    </div>
  );
}

function Stat({
  label,
  value,
  hint,
}: {
  label: string;
  value: string;
  hint?: string;
}) {
  return (
    <div className="bg-card rounded-xl border p-3">
      <p className="text-muted-foreground text-xs">{label}</p>
      <p className="mt-1 truncate text-lg font-semibold tabular-nums">
        {value}
      </p>
      {hint && <p className="text-muted-foreground truncate text-xs">{hint}</p>}
    </div>
  );
}

function Timeline({ uuid }: { uuid: string }) {
  const { t } = useLocale();
  const scope = useContactsScope();
  const [filter, setFilter] = useState<Filter>("all");
  const [note, setNote] = useState("");
  const timeline = useCompanyTimeline(uuid, FILTER_KINDS[filter]);
  const notes = useCompanyNoteMutations(uuid);
  const items = timeline.data?.pages.flatMap((p) => p.items) ?? [];
  return (
    <>
      <div className="flex flex-wrap items-center gap-2">
        <h2 className="font-medium">{t("contacts.account.timeline")}</h2>
        <ToggleGroup
          type="single"
          size="sm"
          variant="outline"
          className="ml-auto"
          value={filter}
          onValueChange={(v) => v && setFilter(v as Filter)}
        >
          {(Object.keys(FILTER_KINDS) as Filter[]).map((f) => (
            <ToggleGroupItem key={f} value={f} className="px-2.5 text-xs">
              {t(`contacts.account.filter.${f}`)}
            </ToggleGroupItem>
          ))}
        </ToggleGroup>
      </div>
      {scope.canWrite && (
        <form
          className="grid gap-2"
          onSubmit={(e) => {
            e.preventDefault();
            notes.add.mutate(note.trim(), { onSuccess: () => setNote("") });
          }}
        >
          <Textarea
            rows={2}
            value={note}
            maxLength={10000}
            placeholder={t("contacts.account.note_placeholder")}
            onChange={(e) => setNote(e.target.value)}
          />
          <Button
            type="submit"
            size="sm"
            className="justify-self-end"
            disabled={!note.trim() || notes.add.isPending}
          >
            {t("contacts.account.add_note")}
          </Button>
        </form>
      )}
      {timeline.isLoading && <Skeleton className="h-40 rounded-lg" />}
      {!timeline.isLoading && items.length === 0 && (
        <p className="text-muted-foreground py-10 text-center text-sm">
          {t("contacts.account.timeline_empty")}
        </p>
      )}
      <ol className="relative flex flex-col">
        {items.map((it, i) => (
          <TimelineRow
            key={`${it.kind}-${it.ref ?? ""}-${it.occurred_at}-${i}`}
            item={it}
            canDelete={scope.canWrite}
            onDeleteNote={(ref) => notes.remove.mutate(ref)}
          />
        ))}
      </ol>
      {timeline.hasNextPage && (
        <Button
          variant="ghost"
          size="sm"
          className="self-center"
          disabled={timeline.isFetchingNextPage}
          onClick={() => void timeline.fetchNextPage()}
        >
          {t("contacts.account.load_more")}
        </Button>
      )}
    </>
  );
}

const KIND_ICON: Record<CompanyTimelineKind, typeof MessageSquare> = {
  conversation_opened: MessageSquare,
  conversation_resolved: CheckCircle2,
  csat: Star,
  note: StickyNote,
  event: Zap,
  revenue: Wallet,
  health: HeartPulse,
};

function TimelineRow({
  item,
  canDelete,
  onDeleteNote,
}: {
  item: CompanyTimelineItem;
  canDelete: boolean;
  onDeleteNote: (uuid: string) => void;
}) {
  const { t, locale } = useLocale();
  const Icon = KIND_ICON[item.kind];
  const d = item.data as Record<string, unknown>;
  const when = new Date(item.occurred_at).toLocaleString(locale, {
    dateStyle: "medium",
    timeStyle: "short",
  });
  let title: React.ReactNode = null;
  let body: React.ReactNode = null;
  switch (item.kind) {
    case "conversation_opened":
    case "conversation_resolved":
      title = (
        <Link
          href={`/inbox?c=${item.ref}`}
          className="underline-offset-2 hover:underline"
        >
          {t(`contacts.account.kind.${item.kind}`, {
            number: String(d.number ?? ""),
          })}
          {item.title ? ` · ${item.title}` : ""}
        </Link>
      );
      body = (
        <span className="text-muted-foreground flex items-center gap-1 text-xs">
          <ChannelGlyph
            type={String(d.channel_type ?? "")}
            className="size-3.5"
          />
          {item.actor}
        </span>
      );
      break;
    case "csat":
      title = t("contacts.account.kind.csat", {
        name: item.actor || t("contacts.account.someone"),
        score: String(d.score ?? ""),
      });
      body = item.body ? <q className="text-sm">{item.body}</q> : null;
      break;
    case "note":
      title = t("contacts.account.kind.note", {
        name: item.actor || t("contacts.account.someone"),
      });
      body = <p className="text-sm whitespace-pre-wrap">{item.body}</p>;
      break;
    case "event": {
      title = (
        <>
          <code className="bg-muted rounded px-1 text-xs">{item.title}</code>
          {item.actor ? ` · ${item.actor}` : ""}
        </>
      );
      const props = Object.entries(
        (d.properties as Record<string, unknown>) ?? {},
      ).slice(0, 6);
      body = props.length ? (
        <div className="flex flex-wrap gap-1">
          {props.map(([k, v]) => (
            <span
              key={k}
              className="bg-muted text-muted-foreground rounded px-1.5 py-0.5 text-[11px]"
            >
              {k}: {typeof v === "object" ? JSON.stringify(v) : String(v)}
            </span>
          ))}
        </div>
      ) : null;
      break;
    }
    case "revenue":
      title = t("contacts.account.kind.revenue", {
        plan: item.title || "—",
        mrr: formatMinor(
          Number(d.mrr_minor ?? 0),
          String(d.currency ?? "TRY"),
          locale,
        ),
      });
      body = (
        <span className="text-muted-foreground text-xs">
          {t(`contacts.revenue.status.${String(d.billing_status ?? "none")}`)}
          {item.actor ? ` · ${item.actor}` : ""}
        </span>
      );
      break;
    case "health":
      title = t("contacts.account.kind.health", {
        from: String(d.previous ?? ""),
        to: String(d.score ?? ""),
      });
      break;
  }
  return (
    <li className="group flex gap-3 border-l py-2.5 pl-4 last:pb-0">
      <span className="bg-background -ml-[1.6rem] flex size-6 shrink-0 items-center justify-center rounded-full border">
        <Icon className="size-3.5" />
      </span>
      <div className="flex min-w-0 flex-1 flex-col gap-1">
        <div className="flex items-start gap-2">
          <p className="min-w-0 flex-1 text-sm font-medium">{title}</p>
          <time className="text-muted-foreground shrink-0 text-xs">{when}</time>
          {item.kind === "note" && canDelete && item.ref && (
            <button
              type="button"
              aria-label={t("contacts.delete")}
              className="text-muted-foreground hover:text-destructive opacity-0 transition-opacity group-hover:opacity-100 focus-visible:opacity-100"
              onClick={() => onDeleteNote(item.ref as string)}
            >
              <Trash2 className="size-3.5" />
            </button>
          )}
        </div>
        {body}
      </div>
    </li>
  );
}

function OpenConversations({ overview }: { overview: CompanyOverview }) {
  const { t, locale } = useLocale();
  const rows = [...overview.open_conversations, ...overview.recently_closed];
  return (
    <section className="bg-card rounded-xl border p-4">
      <h2 className="mb-2 font-medium">
        {t("contacts.account.conversations")}
      </h2>
      {rows.length === 0 && (
        <p className="text-muted-foreground text-sm">
          {t("contacts.account.no_conversations")}
        </p>
      )}
      <ul className="flex flex-col gap-1">
        {rows.slice(0, 12).map((c) => (
          <li key={c.uuid}>
            <Link
              href={`/inbox?c=${c.uuid}`}
              className="hover:bg-muted flex items-center gap-2 rounded-md px-2 py-1.5 text-sm"
            >
              <ChannelGlyph type={c.channel_type} className="size-4" />
              <span className="min-w-0 flex-1 truncate">
                #{c.number} {c.subject || c.last_message_preview}
              </span>
              <span
                className={
                  c.status === "resolved" || c.status === "closed"
                    ? "text-muted-foreground text-xs"
                    : "text-signal-text text-xs font-medium"
                }
              >
                {t(`inbox.status.${c.status}`)}
              </span>
              <span className="text-muted-foreground text-xs tabular-nums">
                {shortAgo(c.last_message_at, locale)}
              </span>
            </Link>
          </li>
        ))}
      </ul>
    </section>
  );
}

function CompanyContacts({ uuid }: { uuid: string }) {
  const { t } = useLocale();
  const list = useContactList({ company: uuid, limit: 20, offset: 0 });
  const items = list.data?.items ?? [];
  return (
    <section className="bg-card rounded-xl border p-4">
      <div className="mb-2 flex items-center">
        <h2 className="font-medium">{t("contacts.title")}</h2>
        <Link
          href={`/contacts?company=${uuid}`}
          className="text-muted-foreground hover:text-foreground ml-auto text-xs"
        >
          {t("contacts.account.all_contacts", {
            count: list.data?.total ?? 0,
          })}
        </Link>
      </div>
      {!list.isLoading && items.length === 0 && (
        <p className="text-muted-foreground text-sm">
          {t("contacts.account.no_contacts")}
        </p>
      )}
      <ul className="flex flex-col gap-1.5">
        {items.map((c) => (
          <li key={c.uuid} className="min-w-0 text-sm">
            <p className="truncate font-medium">{c.name || c.email}</p>
            <p className="text-muted-foreground truncate text-xs">
              {[c.job_title, c.email ?? c.phone].filter(Boolean).join(" · ")}
            </p>
          </li>
        ))}
      </ul>
    </section>
  );
}

function CustomFieldsCard({ values }: { values: Record<string, unknown> }) {
  const { t } = useLocale();
  const defs = useCustomFieldDefinitions({ entityType: "company" });
  const active = (defs.data ?? []).filter((d) => !d.archived);
  if (active.length === 0) return null;
  return (
    <section className="bg-card rounded-xl border p-4">
      <h2 className="mb-2 font-medium">
        {t("contacts.account.custom_fields")}
      </h2>
      <dl className="grid gap-2 text-sm">
        {active.map((d) => (
          <div key={d.uuid} className="grid grid-cols-2 gap-2">
            <dt className="text-muted-foreground truncate">{d.label}</dt>
            <dd className="min-w-0">
              <CustomFieldValue definition={d} value={values?.[d.key]} />
            </dd>
          </div>
        ))}
      </dl>
    </section>
  );
}
