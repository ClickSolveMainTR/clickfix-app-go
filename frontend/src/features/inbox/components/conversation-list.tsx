"use client";

import { memo, useEffect, useRef } from "react";
import { useVirtualizer } from "@tanstack/react-virtual";
import {
  Clock3,
  Flame,
  Inbox as InboxIcon,
  StickyNote,
  TextSearch,
} from "lucide-react";

import { Skeleton } from "@/components/ui/skeleton";
import { BrandBadge } from "@/features/brands/components/brand-badge";
import { StatusDot } from "@/features/conversation-statuses/components/status-dot";
import { SlaBadge } from "@/features/sla/components/sla-badge";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import {
  contactLabel,
  initials,
  isWaitingStatus,
  shortAgo,
  shortUntil,
} from "../lib/format";
import type { Conversation } from "../types";
import { ChannelGlyph } from "./channel-glyph";

const ROW_HEIGHT = 76;

type Props = {
  items: Conversation[];
  activeUuid: string | null;
  loading: boolean;
  hasMore: boolean;
  loadingMore: boolean;
  onLoadMore: () => void;
  onSelect: (uuid: string) => void;
  /** Bulk selection (CF-51). */
  selected?: ReadonlySet<string>;
  onToggle?: (uuid: string) => void;
};

/** Virtualized conversation list: thousands of rows stay smooth. */
export function ConversationList({
  items,
  activeUuid,
  loading,
  hasMore,
  loadingMore,
  onLoadMore,
  onSelect,
  selected,
  onToggle,
}: Props) {
  const { t } = useLocale();
  const parentRef = useRef<HTMLDivElement>(null);
  // TanStack Virtual returns functions that React Compiler cannot memoize safely.
  // eslint-disable-next-line react-hooks/incompatible-library -- library API
  const virtualizer = useVirtualizer({
    count: items.length + (hasMore ? 1 : 0),
    getScrollElement: () => parentRef.current,
    estimateSize: () => ROW_HEIGHT,
    overscan: 8,
  });
  const virtualItems = virtualizer.getVirtualItems();
  const last = virtualItems[virtualItems.length - 1];

  useEffect(() => {
    if (last && last.index >= items.length - 1 && hasMore && !loadingMore) {
      onLoadMore();
    }
  }, [last, items.length, hasMore, loadingMore, onLoadMore]);

  // Keep the keyboard selection in view.
  const activeIndex = items.findIndex((c) => c.uuid === activeUuid);
  useEffect(() => {
    if (activeIndex >= 0)
      virtualizer.scrollToIndex(activeIndex, { align: "auto" });
  }, [activeIndex, virtualizer]);

  if (loading && items.length === 0) {
    return (
      <div className="flex flex-col gap-1 p-2" aria-busy>
        {Array.from({ length: 8 }).map((_, i) => (
          <div key={i} className="flex gap-3 rounded-lg p-3">
            <Skeleton className="size-9 rounded-full" />
            <div className="flex flex-1 flex-col gap-2">
              <Skeleton className="h-3 w-1/2" />
              <Skeleton className="h-3 w-4/5" />
            </div>
          </div>
        ))}
      </div>
    );
  }

  if (items.length === 0) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-2 p-8 text-center">
        <div className="bg-muted text-muted-foreground flex size-10 items-center justify-center rounded-full">
          <InboxIcon className="size-5" />
        </div>
        <p className="text-sm font-medium">{t("inbox.list.empty_title")}</p>
        <p className="text-muted-foreground text-xs">
          {t("inbox.list.empty_description")}
        </p>
      </div>
    );
  }

  return (
    <div
      ref={parentRef}
      className="h-full overflow-y-auto overscroll-contain"
      role="listbox"
      aria-label={t("inbox.list.label")}
      aria-activedescendant={activeUuid ? `conv-${activeUuid}` : undefined}
    >
      <div
        className="relative w-full"
        style={{ height: virtualizer.getTotalSize() }}
      >
        {virtualItems.map((row) => {
          const c = items[row.index];
          return (
            <div
              key={row.key}
              className="absolute top-0 left-0 w-full px-1.5"
              style={{
                height: row.size,
                transform: `translateY(${row.start}px)`,
              }}
            >
              {c ? (
                <div className="group/row relative h-full">
                  <ConversationRow
                    conversation={c}
                    active={c.uuid === activeUuid}
                    onSelect={onSelect}
                  />
                  {onToggle && (
                    <input
                      type="checkbox"
                      aria-label={t("inbox.bulk.select", {
                        name: contactLabel(c),
                      })}
                      checked={selected?.has(c.uuid) ?? false}
                      onChange={() => onToggle(c.uuid)}
                      className={cn(
                        "accent-foreground absolute top-[18px] left-[18px] size-4 cursor-pointer transition-opacity",
                        selected && selected.size > 0
                          ? "opacity-100"
                          : "opacity-0 group-hover/row:opacity-100 focus-visible:opacity-100",
                      )}
                    />
                  )}
                </div>
              ) : (
                <div className="flex h-full items-center justify-center">
                  <Skeleton className="h-3 w-24" />
                </div>
              )}
            </div>
          );
        })}
      </div>
    </div>
  );
}

const ConversationRow = memo(function ConversationRow({
  conversation: c,
  active,
  onSelect,
}: {
  conversation: Conversation;
  active: boolean;
  onSelect: (uuid: string) => void;
}) {
  const { t, locale } = useLocale();
  const unread = c.unread_count > 0;
  const name = contactLabel(c);
  return (
    <button
      type="button"
      id={`conv-${c.uuid}`}
      role="option"
      aria-selected={active}
      onClick={() => onSelect(c.uuid)}
      className={cn(
        "group relative flex h-[72px] w-full items-start gap-3 rounded-lg px-3 py-2.5 text-left transition-colors",
        "focus-visible:ring-ring outline-none focus-visible:ring-2",
        active ? "bg-accent text-accent-foreground" : "hover:bg-muted/60",
      )}
    >
      {active && (
        <span
          className="bg-signal absolute top-3 bottom-3 left-0 w-0.5 rounded-full"
          aria-hidden
        />
      )}
      <div className="relative shrink-0">
        <div className="bg-muted text-muted-foreground flex size-9 items-center justify-center rounded-full text-xs font-semibold">
          {initials(name)}
        </div>
        <ChannelGlyph
          type={c.channel_type}
          className="bg-background absolute -right-1 -bottom-1 size-4 rounded-full p-0.5"
        />
      </div>
      <div className="flex min-w-0 flex-1 flex-col gap-0.5">
        <div className="flex items-center gap-2">
          <span
            className={cn(
              "truncate text-sm",
              unread ? "font-semibold" : "font-medium",
            )}
          >
            {name}
          </span>
          <BrandBadge brandId={c.brand_id} />
          {c.priority === "urgent" && (
            <Flame
              className="size-3.5 shrink-0 text-red-500"
              aria-label={t("inbox.priority.urgent")}
            />
          )}
          <span className="ml-auto flex shrink-0 items-center gap-1.5">
            <SlaBadge conversation={c} />
            <span className="text-muted-foreground text-[11px] tabular-nums">
              {shortAgo(c.last_message_at, locale)}
            </span>
          </span>
        </div>
        {c.search_match ? (
          <div
            className="flex items-center gap-1.5"
            title={c.search_match.snippet}
          >
            {c.search_match.private ? (
              <StickyNote
                className="size-3 shrink-0 text-amber-500"
                aria-label={t("inbox.list.match_note")}
              />
            ) : (
              <TextSearch
                className="text-signal size-3 shrink-0"
                aria-label={t("inbox.list.match_message")}
              />
            )}
            <span className="text-foreground truncate text-xs">
              {c.search_match.snippet}
            </span>
          </div>
        ) : (
          <div className="flex items-center gap-1.5">
            {c.last_message_direction === "internal" ? (
              <StickyNote className="size-3 shrink-0 text-amber-500" />
            ) : c.last_message_direction === "out" ? (
              <span className="text-muted-foreground shrink-0 text-xs">
                {t("inbox.list.you")}:
              </span>
            ) : null}
            <span
              className={cn(
                "truncate text-xs",
                unread ? "text-foreground" : "text-muted-foreground",
              )}
            >
              {c.subject && c.channel_type === "email"
                ? c.subject
                : c.last_message_preview || "—"}
            </span>
            {unread && (
              <span className="bg-signal text-signal-foreground ml-auto min-w-5 shrink-0 rounded-full px-1.5 text-center text-[10px] leading-5 font-semibold tabular-nums">
                {c.unread_count > 99 ? "99+" : c.unread_count}
              </span>
            )}
          </div>
        )}
        <div className="text-muted-foreground flex items-center gap-2 text-[11px]">
          <span className="tabular-nums">#{c.number}</span>
          {c.assignee ? (
            <span className="truncate">{c.assignee.name}</span>
          ) : (
            <span className="italic">{t("inbox.list.unassigned")}</span>
          )}
          {c.custom_status && (
            <span className="flex min-w-0 items-center gap-1">
              <StatusDot color={c.custom_status.color} />
              <span className="truncate">{c.custom_status.name}</span>
            </span>
          )}
          {c.status === "snoozed" && c.snoozed_until && (
            <span className="flex items-center gap-0.5">
              <Clock3 className="size-3" />
              {shortUntil(c.snoozed_until, locale)}
            </span>
          )}
          {c.waiting_since && isWaitingStatus(c.status) && (
            <span className="ml-auto flex items-center gap-0.5 text-amber-600 dark:text-amber-400">
              {t("inbox.list.waiting", {
                time: shortAgo(c.waiting_since, locale),
              })}
            </span>
          )}
        </div>
      </div>
    </button>
  );
});
