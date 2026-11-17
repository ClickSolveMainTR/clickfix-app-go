"use client";

import Link from "next/link";
import {
  forwardRef,
  useEffect,
  useImperativeHandle,
  useMemo,
  useRef,
  useState,
} from "react";
import { Filter, Loader2, Search, Trash2, X } from "lucide-react";

import { EmptyState } from "@/components/common/empty-state";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { DatePicker } from "@/components/ui/date-picker";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { Textarea } from "@/components/ui/textarea";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { routes } from "@/config/routes";
import { ChannelGlyph } from "@/features/inbox/components/channel-glyph";
import { useDebounce } from "@/hooks/use-debounce";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import {
  useContactNoteMutations,
  useContactTimeline,
} from "../../hooks/use-contact-profile";
import { useContactsScope } from "../../hooks/use-contacts";
import type {
  ContactTimelineItem,
  ContactTimelineKind,
} from "../../services/profile.service";
import {
  KIND_META,
  TIMELINE_GROUPS,
  dateTimeOf,
  dayKey,
  dayLabel,
  timeOf,
  type TimelineGroup,
} from "./lib";

export type ContactTimelineHandle = { focusComposer: () => void };

/**
 * Unified contact timeline (CF-221): every source in one stream with kind
 * chips, literal search and a date range; older pages load on scroll.
 */
export const ContactTimeline = forwardRef<
  ContactTimelineHandle,
  { uuid: string; kinds: ContactTimelineKind[] }
>(function ContactTimeline({ uuid, kinds }, ref) {
  const { t, locale } = useLocale();
  const scope = useContactsScope();
  const [groups, setGroups] = useState<TimelineGroup[]>([]);
  const [search, setSearch] = useState("");
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const q = useDebounce(search.trim(), 300);
  const composer = useRef<HTMLTextAreaElement>(null);
  useImperativeHandle(ref, () => ({
    focusComposer: () => {
      composer.current?.scrollIntoView({ behavior: "smooth", block: "center" });
      composer.current?.focus({ preventScroll: true });
    },
  }));

  // Only chips whose kinds the member may see.
  const available = useMemo(
    () =>
      (Object.keys(TIMELINE_GROUPS) as TimelineGroup[]).filter((g) =>
        TIMELINE_GROUPS[g].some((k) => kinds.includes(k)),
      ),
    [kinds],
  );
  const selectedKinds = useMemo(
    () =>
      groups.flatMap((g) =>
        TIMELINE_GROUPS[g].filter((k) => kinds.includes(k)),
      ) as ContactTimelineKind[],
    [groups, kinds],
  );
  const query = {
    kinds: selectedKinds,
    q: q || undefined,
    from: from || undefined,
    to: to || undefined,
  };
  const timeline = useContactTimeline(uuid, query);
  const items = useMemo(
    () => timeline.data?.pages.flatMap((p) => p.items) ?? [],
    [timeline.data],
  );
  const filtered = groups.length > 0 || Boolean(q) || Boolean(from || to);
  const reset = () => {
    setGroups([]);
    setSearch("");
    setFrom("");
    setTo("");
  };

  // Infinite scroll: load the next page when the sentinel is visible.
  const sentinel = useRef<HTMLDivElement>(null);
  const { hasNextPage, isFetchingNextPage, fetchNextPage } = timeline;
  useEffect(() => {
    const el = sentinel.current;
    if (!el || !hasNextPage || typeof IntersectionObserver === "undefined")
      return;
    const io = new IntersectionObserver(
      (entries) => {
        if (entries[0]?.isIntersecting && !isFetchingNextPage)
          void fetchNextPage();
      },
      { rootMargin: "240px" },
    );
    io.observe(el);
    return () => io.disconnect();
  }, [hasNextPage, isFetchingNextPage, fetchNextPage]);

  const days = useMemo(() => {
    const out: { key: string; items: ContactTimelineItem[] }[] = [];
    for (const it of items) {
      const key = dayKey(it.occurred_at);
      const last = out[out.length - 1];
      if (last?.key === key) last.items.push(it);
      else out.push({ key, items: [it] });
    }
    return out;
  }, [items]);

  return (
    <div className="flex flex-col gap-4">
      {scope.canWrite && <NoteComposer uuid={uuid} ref={composer} />}

      <div className="flex flex-col gap-3" role="search">
        <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
          <div className="relative min-w-0 flex-1">
            <Label htmlFor="contact-timeline-search" className="sr-only">
              {t("contacts.profile.timeline.search")}
            </Label>
            <Search className="text-muted-foreground pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2" />
            <Input
              id="contact-timeline-search"
              type="search"
              value={search}
              maxLength={200}
              placeholder={t("contacts.profile.timeline.search")}
              className="pl-8"
              onChange={(e) => setSearch(e.target.value)}
            />
          </div>
          <div className="grid grid-cols-2 gap-2 sm:flex">
            <DatePicker
              id="contact-timeline-from"
              value={from}
              onChange={setFrom}
              placeholder={t("contacts.profile.timeline.from")}
              className="sm:w-40"
              align="end"
            />
            <DatePicker
              id="contact-timeline-to"
              value={to}
              onChange={setTo}
              placeholder={t("contacts.profile.timeline.to")}
              className="sm:w-40"
              align="end"
            />
          </div>
        </div>
        <div className="flex items-start gap-2">
          <Filter
            aria-hidden
            className="text-muted-foreground mt-1.5 size-4 shrink-0"
          />
          <ToggleGroup
            type="multiple"
            size="sm"
            variant="outline"
            aria-label={t("contacts.profile.timeline.filter")}
            className="flex flex-1 flex-wrap justify-start gap-1.5 [&>*]:rounded-full! [&>*]:border!"
            value={groups}
            onValueChange={(v) => setGroups(v as TimelineGroup[])}
          >
            {available.map((g) => (
              <ToggleGroupItem
                key={g}
                value={g}
                className="data-[state=on]:bg-signal-soft data-[state=on]:text-signal-text h-7 px-3 text-xs"
              >
                {t(`contacts.profile.timeline.group.${g}`)}
              </ToggleGroupItem>
            ))}
          </ToggleGroup>
          {filtered && (
            <Button
              size="sm"
              variant="ghost"
              className="h-7 shrink-0 px-2 text-xs"
              onClick={reset}
            >
              <X className="size-3.5" />
              {t("contacts.profile.timeline.clear")}
            </Button>
          )}
        </div>
      </div>

      {timeline.isLoading ? (
        <TimelineSkeleton />
      ) : items.length === 0 ? (
        <EmptyState
          title={
            filtered
              ? t("contacts.profile.timeline.no_match")
              : t("contacts.profile.timeline.empty")
          }
          description={
            filtered
              ? t("contacts.profile.timeline.no_match_hint")
              : t("contacts.profile.timeline.empty_hint")
          }
          action={
            filtered ? (
              <Button size="sm" variant="outline" onClick={reset}>
                {t("contacts.profile.timeline.clear")}
              </Button>
            ) : undefined
          }
        />
      ) : (
        <div
          className={cn(
            "flex flex-col gap-5 transition-opacity",
            timeline.isPlaceholderData && "opacity-60",
          )}
          aria-busy={timeline.isFetching}
        >
          {days.map((day) => (
            <section key={day.key} aria-label={dayLabel(day.key, locale, t)}>
              <h3 className="bg-background/90 text-muted-foreground sticky top-0 z-10 mb-1 py-1 text-xs font-semibold backdrop-blur">
                {dayLabel(day.key, locale, t)}
              </h3>
              <ol className="flex flex-col">
                {day.items.map((it, i) => (
                  <TimelineRow
                    key={`${it.kind}-${it.ref ?? ""}-${it.occurred_at}-${i}`}
                    contactUuid={uuid}
                    item={it}
                    canDelete={scope.canWrite}
                  />
                ))}
              </ol>
            </section>
          ))}
          <div ref={sentinel} />
          {timeline.hasNextPage ? (
            <Button
              variant="ghost"
              size="sm"
              className="self-center"
              disabled={timeline.isFetchingNextPage}
              onClick={() => void timeline.fetchNextPage()}
            >
              {timeline.isFetchingNextPage && (
                <Loader2 className="size-4 animate-spin" />
              )}
              {t("contacts.account.load_more")}
            </Button>
          ) : (
            <p className="text-muted-foreground text-center text-xs">
              {t("contacts.profile.timeline.end")}
            </p>
          )}
        </div>
      )}
    </div>
  );
});

const NoteComposer = forwardRef<HTMLTextAreaElement, { uuid: string }>(
  function NoteComposer({ uuid }, ref) {
    const { t } = useLocale();
    const notes = useContactNoteMutations(uuid);
    const [note, setNote] = useState("");
    const submit = () => {
      if (!note.trim() || notes.add.isPending) return;
      notes.add.mutate(note.trim(), { onSuccess: () => setNote("") });
    };
    return (
      <form
        className="bg-muted/30 focus-within:border-signal/50 grid gap-2 rounded-xl border p-3 transition-colors"
        onSubmit={(e) => {
          e.preventDefault();
          submit();
        }}
      >
        <Label htmlFor="contact-note" className="sr-only">
          {t("contacts.profile.action.add_note")}
        </Label>
        <Textarea
          id="contact-note"
          ref={ref}
          rows={2}
          value={note}
          maxLength={10000}
          className="bg-background resize-y"
          placeholder={t("contacts.profile.note_placeholder")}
          onChange={(e) => setNote(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
              e.preventDefault();
              submit();
            }
          }}
        />
        <div className="flex items-center gap-2">
          <p className="text-muted-foreground hidden flex-1 text-xs sm:block">
            {t("contacts.profile.note_hint")}
          </p>
          <Button
            type="submit"
            size="sm"
            className="ml-auto"
            disabled={!note.trim() || notes.add.isPending}
          >
            {t("contacts.account.add_note")}
          </Button>
        </div>
      </form>
    );
  },
);

function TimelineSkeleton() {
  return (
    <div className="flex flex-col gap-4" aria-hidden>
      {[0, 1, 2, 3].map((i) => (
        <div key={i} className="flex gap-3">
          <Skeleton className="size-7 shrink-0 rounded-full" />
          <div className="grid flex-1 gap-2">
            <Skeleton className="h-4 w-2/3" />
            <Skeleton className="h-3 w-1/3" />
          </div>
        </div>
      ))}
    </div>
  );
}

function str(v: unknown) {
  return v == null ? "" : String(v);
}

function TimelineRow({
  contactUuid,
  item,
  canDelete,
}: {
  contactUuid: string;
  item: ContactTimelineItem;
  canDelete: boolean;
}) {
  const { t, locale } = useLocale();
  const notes = useContactNoteMutations(contactUuid);
  const { icon: Icon, tone } = KIND_META[item.kind];
  const d = (item.data ?? {}) as Record<string, unknown>;
  const someone = item.actor || t("contacts.account.someone");
  let title: React.ReactNode = null;
  let body: React.ReactNode = null;
  let meta: React.ReactNode = null;
  const conversationLink = (uuid: unknown, label: React.ReactNode) =>
    uuid ? (
      <Link
        href={`${routes.cms.inbox}?c=${str(uuid)}`}
        className="hover:text-signal-text underline-offset-2 hover:underline"
      >
        {label}
      </Link>
    ) : (
      label
    );

  switch (item.kind) {
    case "conversation_opened":
    case "conversation_resolved":
      title = conversationLink(
        item.ref,
        <>
          {t(`contacts.account.kind.${item.kind}`, { number: str(d.number) })}
          {item.title ? ` · ${item.title}` : ""}
        </>,
      );
      meta = (
        <span className="inline-flex items-center gap-1">
          <ChannelGlyph type={str(d.channel_type)} className="size-3.5" />
          {t(`channels.types.${str(d.channel_type)}`)}
        </span>
      );
      break;
    case "csat":
      title = conversationLink(
        d.conversation_uuid,
        t("contacts.profile.kind.csat", {
          score: str(d.score),
          number: str(d.number),
        }),
      );
      body = item.body ? <q className="text-sm">{item.body}</q> : null;
      break;
    case "survey":
      title = conversationLink(
        d.conversation_uuid,
        t(`contacts.profile.kind.survey_${str(d.survey_kind) || "nps"}`, {
          score: str(d.score),
        }),
      );
      body = item.body ? <q className="text-sm">{item.body}</q> : null;
      break;
    case "note":
      title = t("contacts.account.kind.note", { name: someone });
      body = (
        <p className="bg-muted/40 rounded-lg px-3 py-2 text-sm whitespace-pre-wrap">
          {item.body}
        </p>
      );
      break;
    case "appointment":
      title = item.ref ? (
        <Link
          href={routes.cms.calendar.appointment(item.ref)}
          className="hover:text-signal-text underline-offset-2 hover:underline"
        >
          {t("contacts.profile.kind.appointment", {
            title: item.title || "—",
          })}
        </Link>
      ) : (
        item.title
      );
      meta = (
        <>
          {d.starts_at ? dateTimeOf(str(d.starts_at), locale) : null}
          <Badge variant="outline" className="h-5 px-1.5 text-[10px]">
            {t(`contacts.profile.appointment_status.${str(d.status)}`)}
          </Badge>
          {item.actor ? <span>{item.actor}</span> : null}
        </>
      );
      body = item.body ? (
        <p className="text-muted-foreground text-sm">{item.body}</p>
      ) : null;
      break;
    case "consent":
      title = t(
        d.granted
          ? "contacts.profile.kind.consent_granted"
          : "contacts.profile.kind.consent_revoked",
        { purpose: item.title },
      );
      meta = (
        <>
          <span>
            {t("contacts.profile.consent_version", {
              version: str(d.notice_version),
            })}
          </span>
          <span>{str(d.source)}</span>
          {item.actor ? <span>{item.actor}</span> : null}
        </>
      );
      break;
    case "privacy_request":
      title = t(`contacts.profile.kind.privacy_${str(d.request_kind)}`, {
        name: someone,
      });
      meta = (
        <Badge variant="outline" className="h-5 px-1.5 text-[10px]">
          {t(`contacts.profile.request_status.${str(d.status)}`)}
        </Badge>
      );
      break;
    case "event": {
      title = (
        <code className="bg-muted rounded px-1.5 py-0.5 text-xs">
          {item.title}
        </code>
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
      meta = d.source ? <span>{str(d.source)}</span> : null;
      break;
    }
    case "feedback":
      title = t(
        d.feedback_kind === "comment"
          ? "contacts.profile.kind.feedback_comment"
          : "contacts.profile.kind.feedback_vote",
        { title: item.title },
      );
      body = item.body ? <q className="text-sm">{item.body}</q> : null;
      meta = (
        <Badge variant="outline" className="h-5 px-1.5 text-[10px]">
          {t(`contacts.profile.feature_status.${str(d.status)}`)}
        </Badge>
      );
      break;
    case "forum":
      title = (
        <Link
          href={routes.cms.forum}
          className="hover:text-signal-text underline-offset-2 hover:underline"
        >
          {t(
            d.is_topic
              ? "contacts.profile.kind.forum_topic"
              : "contacts.profile.kind.forum_reply",
            { title: item.title || "—" },
          )}
        </Link>
      );
      body = item.body ? (
        <p className="text-muted-foreground line-clamp-2 text-sm">
          {item.body}
        </p>
      ) : null;
      meta =
        d.status && d.status !== "published" ? (
          <Badge variant="warning" className="h-5 px-1.5 text-[10px]">
            {t(`contacts.profile.forum_status.${str(d.status)}`)}
          </Badge>
        ) : null;
      break;
    case "status_notice":
      title = t("contacts.profile.kind.status_notice", { title: item.title });
      meta = <span>{str(d.severity)}</span>;
      break;
    case "system":
      title = t(`contacts.profile.system.${item.title.replace(".", "_")}`, {
        name: someone,
        channel: t(`channels.types.${str(d.channel_type)}`),
      });
      break;
  }

  return (
    <li className="group relative flex gap-3 pb-4 last:pb-1">
      <span
        aria-hidden
        className="bg-border absolute top-8 bottom-0 left-3.5 w-px group-last:hidden"
      />
      <span
        className={cn(
          "relative flex size-7 shrink-0 items-center justify-center rounded-full",
          tone,
        )}
      >
        <Icon className="size-3.5" aria-hidden />
      </span>
      <div className="flex min-w-0 flex-1 flex-col gap-1 pt-0.5">
        <div className="flex items-start gap-2">
          <p className="min-w-0 flex-1 text-sm font-medium break-words">
            <span className="sr-only">
              {t(`contacts.profile.kind_label.${item.kind}`)}:{" "}
            </span>
            {title}
          </p>
          <time
            dateTime={item.occurred_at}
            title={dateTimeOf(item.occurred_at, locale)}
            className="text-muted-foreground shrink-0 text-xs tabular-nums"
          >
            {timeOf(item.occurred_at, locale)}
          </time>
          {item.kind === "note" && canDelete && item.ref && (
            <button
              type="button"
              aria-label={t("contacts.profile.delete_note")}
              className="text-muted-foreground hover:text-destructive focus-visible:ring-ring rounded opacity-0 outline-none group-hover:opacity-100 focus-visible:opacity-100 focus-visible:ring-2"
              disabled={notes.remove.isPending}
              onClick={() => notes.remove.mutate(item.ref as string)}
            >
              <Trash2 className="size-3.5" />
            </button>
          )}
        </div>
        {meta && (
          <div className="text-muted-foreground flex flex-wrap items-center gap-x-2 gap-y-1 text-xs">
            {meta}
          </div>
        )}
        {body}
      </div>
    </li>
  );
}
