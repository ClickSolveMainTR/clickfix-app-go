"use client";

import { memo, useEffect, useLayoutEffect, useRef, useState } from "react";
import { useVirtualizer } from "@tanstack/react-virtual";
import {
  AlertCircle,
  ArrowDown,
  Check,
  CheckCheck,
  Clock,
  FileText,
  Loader2,
  Lock,
  MoreHorizontal,
  Pencil,
  RotateCw,
  Split,
} from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { Skeleton } from "@/components/ui/skeleton";
import { cn, fileSize } from "@/lib/utils";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";

import { useMessageEdits } from "../hooks/use-inbox";
import { initials } from "../lib/format";
import { canEditMessage } from "../lib/message-edit";
import { inboxService } from "../services/inbox.service";
import type { OptimisticMessage } from "../types";
import { MessageContent } from "./message-content";

type Props = {
  conversationUuid: string;
  items: OptimisticMessage[];
  loading: boolean;
  hasOlder: boolean;
  loadingOlder: boolean;
  onLoadOlder: () => void;
  onRetry: (message: string) => void;
  /** Splits a message off into a new conversation (CF-98); omitted without permission. */
  onSplit?: (message: OptimisticMessage) => void;
  /** Edits the agent's own reply (CF-248); omitted without reply permission. */
  onEdit?: (message: OptimisticMessage) => void;
};

const NEAR_BOTTOM_PX = 120;

/**
 * Virtualized, bottom-anchored message timeline. New messages keep the
 * view pinned to the bottom unless the agent scrolled up to read history;
 * then a "new messages" pill appears.
 */
export function MessageTimeline({
  conversationUuid,
  items,
  loading,
  hasOlder,
  loadingOlder,
  onLoadOlder,
  onRetry,
  onSplit,
  onEdit,
}: Props) {
  const { t } = useLocale();
  const { user } = useAuth();
  const parentRef = useRef<HTMLDivElement>(null);
  const pinned = useRef(true);
  const [unseen, setUnseen] = useState(0);
  const prevCount = useRef(0);
  const prevFirst = useRef<string | undefined>(undefined);

  // TanStack Virtual returns functions that React Compiler cannot memoize safely.
  // eslint-disable-next-line react-hooks/incompatible-library -- library API
  const virtualizer = useVirtualizer({
    count: items.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => 72,
    overscan: 10,
    getItemKey: (i) => items[i]?.client_id ?? items[i]?.uuid ?? i,
  });

  // A new conversation starts at the bottom.
  useLayoutEffect(() => {
    pinned.current = true;
    prevCount.current = 0;
    prevFirst.current = undefined;
    setUnseen(0);
  }, [conversationUuid]);

  useLayoutEffect(() => {
    const count = items.length;
    const first = items[0]?.uuid;
    if (count === 0) return;
    const olderLoaded =
      prevFirst.current !== undefined && first !== prevFirst.current;
    const added = count - prevCount.current;
    if (prevCount.current === 0 || (pinned.current && !olderLoaded)) {
      virtualizer.scrollToIndex(count - 1, { align: "end" });
    } else if (olderLoaded && added > 0) {
      // Keep the reading position when history is prepended.
      virtualizer.scrollToIndex(added, { align: "start" });
    } else if (added > 0) {
      setUnseen((n) => n + added);
    }
    prevCount.current = count;
    prevFirst.current = first;
  }, [items, virtualizer]);

  useEffect(() => {
    const el = parentRef.current;
    if (!el) return;
    const onScroll = () => {
      const distance = el.scrollHeight - el.scrollTop - el.clientHeight;
      pinned.current = distance < NEAR_BOTTOM_PX;
      if (pinned.current) setUnseen(0);
      if (el.scrollTop < 200 && hasOlder && !loadingOlder) onLoadOlder();
    };
    el.addEventListener("scroll", onScroll, { passive: true });
    return () => el.removeEventListener("scroll", onScroll);
  }, [hasOlder, loadingOlder, onLoadOlder]);

  if (loading && items.length === 0) {
    return (
      <div className="flex flex-1 flex-col justify-end gap-4 p-6">
        <Skeleton className="h-12 w-2/3 rounded-2xl" />
        <Skeleton className="ml-auto h-10 w-1/2 rounded-2xl" />
        <Skeleton className="h-16 w-3/5 rounded-2xl" />
      </div>
    );
  }

  return (
    <div className="relative min-h-0 flex-1">
      <div
        ref={parentRef}
        className="h-full overflow-y-auto overscroll-contain px-4 md:px-6"
        role="log"
        aria-live="polite"
        aria-label={t("inbox.timeline.label")}
      >
        {loadingOlder && (
          <div className="text-muted-foreground flex justify-center py-3">
            <Loader2 className="size-4 animate-spin" />
          </div>
        )}
        <div
          className="relative w-full"
          style={{ height: virtualizer.getTotalSize() }}
        >
          {virtualizer.getVirtualItems().map((row) => {
            const m = items[row.index];
            if (!m) return null;
            const prev = items[row.index - 1];
            const grouped =
              prev != null &&
              prev.kind !== "event" &&
              m.kind !== "event" &&
              prev.direction === m.direction &&
              prev.sender.uuid === m.sender.uuid &&
              new Date(m.created_at).getTime() -
                new Date(prev.created_at).getTime() <
                5 * 60_000;
            const dayBreak =
              !prev ||
              new Date(prev.created_at).toDateString() !==
                new Date(m.created_at).toDateString();
            return (
              <div
                key={row.key}
                data-index={row.index}
                ref={virtualizer.measureElement}
                className="absolute top-0 left-0 w-full"
                style={{ transform: `translateY(${row.start}px)` }}
              >
                {dayBreak && <DayDivider iso={m.created_at} />}
                <MessageItem
                  message={m}
                  grouped={grouped && !dayBreak}
                  onRetry={onRetry}
                  onSplit={onSplit}
                  onEdit={
                    onEdit && canEditMessage(m, user?.uuid) ? onEdit : undefined
                  }
                />
              </div>
            );
          })}
        </div>
      </div>
      {unseen > 0 && (
        <Button
          size="sm"
          className="absolute bottom-3 left-1/2 -translate-x-1/2 rounded-full shadow-md"
          onClick={() => {
            virtualizer.scrollToIndex(items.length - 1, { align: "end" });
            setUnseen(0);
          }}
        >
          <ArrowDown className="size-3.5" />
          {t("inbox.timeline.new_messages", { count: unseen })}
        </Button>
      )}
    </div>
  );
}

function DayDivider({ iso }: { iso: string }) {
  const { locale } = useLocale();
  const d = new Date(iso);
  const label = d.toLocaleDateString(locale === "tr" ? "tr-TR" : "en-US", {
    weekday: "long",
    day: "numeric",
    month: "long",
  });
  return (
    <div className="flex items-center gap-3 py-4" role="separator">
      <div className="bg-border h-px flex-1" />
      <span className="text-muted-foreground text-[11px] font-medium tracking-wide uppercase">
        {label}
      </span>
      <div className="bg-border h-px flex-1" />
    </div>
  );
}

const MessageItem = memo(function MessageItem({
  message: m,
  grouped,
  onRetry,
  onSplit,
  onEdit,
}: {
  message: OptimisticMessage;
  grouped: boolean;
  onRetry: (message: string) => void;
  onSplit?: (message: OptimisticMessage) => void;
  onEdit?: (message: OptimisticMessage) => void;
}) {
  const { t, locale } = useLocale();
  if (m.kind === "event") return <TimelineEvent message={m} />;
  const mine = m.direction === "out";
  const note = m.direction === "internal";
  const time = new Date(m.created_at).toLocaleTimeString(
    locale === "tr" ? "tr-TR" : "en-US",
    {
      hour: "2-digit",
      minute: "2-digit",
    },
  );
  return (
    <div
      className={cn(
        "group/msg flex gap-2.5",
        mine || note ? "flex-row-reverse" : "flex-row",
        grouped ? "pt-0.5" : "pt-3",
      )}
    >
      <div className="w-7 shrink-0">
        {!grouped && (
          <div
            className={cn(
              "flex size-7 items-center justify-center rounded-full text-[10px] font-semibold",
              mine
                ? "bg-ink text-paper"
                : note
                  ? "bg-amber-100 text-amber-900 dark:bg-amber-900/40 dark:text-amber-200"
                  : "bg-muted text-muted-foreground",
            )}
            title={m.sender.name}
          >
            {initials(m.sender.name)}
          </div>
        )}
      </div>
      <div
        className={cn(
          "flex max-w-[78%] min-w-0 flex-col gap-1",
          mine || note ? "items-end" : "items-start",
        )}
      >
        {!grouped && (
          <div className="text-muted-foreground flex items-center gap-1.5 px-1 text-[11px]">
            {note && <Lock className="size-3 text-amber-600" />}
            <span className="text-foreground/80 font-medium">
              {m.sender.name || t(`inbox.sender.${m.sender.type}`)}
            </span>
            {note && <span>· {t("inbox.timeline.private_note")}</span>}
            <span>· {time}</span>
          </div>
        )}
        <div
          className={cn(
            "rounded-2xl px-3.5 py-2 text-sm leading-relaxed break-words whitespace-pre-wrap shadow-xs",
            mine && "bg-ink text-paper rounded-tr-md",
            note &&
              "rounded-tr-md border border-amber-200 bg-amber-50 text-amber-950 dark:border-amber-900/60 dark:bg-amber-950/40 dark:text-amber-100",
            !mine && !note && "bg-card border-border rounded-tl-md border",
            m.optimistic && "opacity-70",
          )}
        >
          {m.attributes &&
            typeof m.attributes.subject === "string" &&
            m.attributes.subject && (
              <div className="mb-1 font-semibold">
                {m.attributes.subject as string}
              </div>
            )}
          {(m.content && mine ? (
            <MessageContent text={m.content} />
          ) : (
            m.content
          )) ||
            (m.attachments.length === 0 ? (
              <span className="italic opacity-70">
                {t("inbox.timeline.unsupported")}
              </span>
            ) : null)}
          {m.attachments.length > 0 && (
            <div className={cn("flex flex-wrap gap-2", m.content && "mt-2")}>
              {m.attachments.map((a) => (
                <button
                  type="button"
                  key={a.uuid}
                  onClick={() => void inboxService.openAttachment(a.uuid)}
                  className="bg-background/80 text-foreground hover:bg-background flex max-w-56 items-center gap-2 rounded-lg border px-2 py-1.5 text-xs"
                >
                  <FileText className="size-4 shrink-0" />
                  <span className="truncate">{a.filename}</span>
                  <span className="text-muted-foreground shrink-0">
                    {fileSize(a.size_bytes)}
                  </span>
                </button>
              ))}
            </div>
          )}
        </div>
        {(mine || m.edited_at) && (
          <div className="flex items-center gap-1.5">
            {m.edited_at && <EditedLabel message={m} />}
            {mine && <DeliveryState message={m} onRetry={onRetry} />}
          </div>
        )}
      </div>
      {(onSplit || onEdit) && !m.optimistic && (
        // Not modal: the item opens a dialog, which then owns focus.
        <DropdownMenu modal={false}>
          <DropdownMenuTrigger asChild>
            <Button
              variant="ghost"
              size="icon"
              className="text-muted-foreground size-6 self-center opacity-0 group-hover/msg:opacity-100 focus:opacity-100 data-[state=open]:opacity-100"
              title={t("inbox.message_menu")}
              aria-label={t("inbox.message_menu")}
            >
              <MoreHorizontal className="size-3.5" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align={mine || note ? "end" : "start"}>
            {onEdit && (
              <DropdownMenuItem onSelect={() => onEdit(m)}>
                <Pencil className="size-3.5" />
                {t("inbox.edit.action")}
              </DropdownMenuItem>
            )}
            {onSplit && (
              <DropdownMenuItem onSelect={() => onSplit(m)}>
                <Split className="size-3.5" />
                {t("inbox.split.action")}
              </DropdownMenuItem>
            )}
          </DropdownMenuContent>
        </DropdownMenu>
      )}
    </div>
  );
});

/**
 * "düzenlendi" under an edited message (CF-248); opens the earlier
 * versions, loaded on demand.
 */
function EditedLabel({ message: m }: { message: OptimisticMessage }) {
  const { t, locale } = useLocale();
  const [open, setOpen] = useState(false);
  const history = useMessageEdits(m.conversation_uuid, m.uuid, open);
  const fmt = (iso: string) =>
    new Date(iso).toLocaleString(locale === "tr" ? "tr-TR" : "en-US", {
      day: "numeric",
      month: "short",
      hour: "2-digit",
      minute: "2-digit",
    });
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <button
          type="button"
          className="text-muted-foreground px-1 text-[11px] italic underline-offset-2 hover:underline"
          title={m.edited_at ? fmt(m.edited_at) : undefined}
        >
          {t("inbox.edit.edited")}
        </button>
      </PopoverTrigger>
      <PopoverContent className="w-80 text-sm" align="end">
        <div className="mb-2 font-medium">{t("inbox.edit.history")}</div>
        {history.isLoading ? (
          <Loader2 className="text-muted-foreground size-4 animate-spin" />
        ) : (
          <ol className="flex max-h-64 flex-col gap-2 overflow-y-auto">
            {(history.data?.items ?? []).map((e, i) => (
              <li key={e.uuid} className="flex flex-col gap-0.5">
                <span className="text-muted-foreground text-[11px]">
                  {i === 0
                    ? t("inbox.edit.original")
                    : t("inbox.edit.version", { n: i + 1 })}{" "}
                  ·{" "}
                  {t("inbox.edit.replaced", {
                    name: e.editor?.name ?? "—",
                    time: fmt(e.edited_at),
                  })}
                </span>
                <span className="bg-muted/50 rounded-md px-2 py-1 text-xs break-words whitespace-pre-wrap">
                  {e.previous_content}
                </span>
              </li>
            ))}
          </ol>
        )}
      </PopoverContent>
    </Popover>
  );
}

function DeliveryState({
  message: m,
  onRetry,
}: {
  message: OptimisticMessage;
  onRetry: (message: string) => void;
}) {
  const { t } = useLocale();
  if (m.status === "failed") {
    return (
      <div className="flex items-center gap-1.5 px-1 text-[11px] text-red-600 dark:text-red-400">
        <AlertCircle className="size-3" />
        <span title={m.error}>{t("inbox.delivery.failed")}</span>
        {!m.optimistic && (
          <button
            type="button"
            className="inline-flex items-center gap-0.5 font-medium underline-offset-2 hover:underline"
            onClick={() => onRetry(m.uuid)}
          >
            <RotateCw className="size-3" />
            {t("inbox.delivery.retry")}
          </button>
        )}
      </div>
    );
  }
  const icon =
    m.status === "read" ? (
      <CheckCheck className="text-signal-text size-3.5" />
    ) : m.status === "delivered" ? (
      <CheckCheck className="size-3.5" />
    ) : m.status === "sent" ? (
      <Check className="size-3.5" />
    ) : (
      <Clock className="size-3" />
    );
  return (
    <div
      className="text-muted-foreground flex items-center gap-1 px-1 text-[11px]"
      title={t(`inbox.delivery.${m.status}`)}
    >
      {icon}
      <span className="sr-only">{t(`inbox.delivery.${m.status}`)}</span>
    </div>
  );
}

/** CF-138 appointment events written into the conversation timeline. */
const APPOINTMENT_EVENT_KEYS: Record<string, string> = {
  appointment_booked: "inbox.events.appointment_booked",
  appointment_rescheduled: "inbox.events.appointment_rescheduled",
  appointment_confirmed: "inbox.events.appointment_confirmed",
  appointment_completed: "inbox.events.appointment_completed",
  appointment_no_show: "inbox.events.appointment_no_show",
  appointment_cancelled: "inbox.events.appointment_cancelled",
};

function TimelineEvent({ message: m }: { message: OptimisticMessage }) {
  const { t } = useLocale();
  const a = (m.attributes ?? {}) as Record<string, string>;
  const actor = a.actor_name || t("inbox.sender.system");
  let text: string;
  switch (a.event) {
    case "assigned":
      text = a.to
        ? a.to === actor
          ? t("inbox.events.took", { actor })
          : t("inbox.events.assigned", { actor, to: a.to })
        : t("inbox.events.unassigned", { actor });
      break;
    case "status_changed": {
      const [status] = (a.to ?? "").split("|");
      text = t("inbox.events.status_changed", {
        actor,
        status: t(`inbox.status.${status}`),
      });
      break;
    }
    case "priority_changed":
      text = t("inbox.events.priority_changed", {
        actor,
        priority: t(`inbox.priority.${a.to}`),
      });
      break;
    case "tag_added":
      text = t("inbox.events.tag_added", { actor, tag: a.to ?? "" });
      break;
    case "tag_removed":
      text = t("inbox.events.tag_removed", { actor, tag: a.to ?? "" });
      break;
    case "team_assigned":
      text = a.to
        ? t("inbox.events.team_assigned", { actor, team: a.to })
        : t("inbox.events.team_removed", { actor });
      break;
    case "subject_changed":
      text = t("inbox.events.subject_changed", { actor });
      break;
    case "department_transferred": {
      const to = a.to || t("inbox.events.department_organization_wide");
      const base =
        a.source === "customer"
          ? t("inbox.events.department_picked", { department: to })
          : a.source === "ai" && !a.from
            ? t("inbox.events.department_predicted", { department: to })
            : t("inbox.events.department_transferred", {
                actor:
                  a.source === "ai" ? t("inbox.events.department_ai") : actor,
                department: to,
              });
      text = a.reason ? `${base}: ${a.reason}` : base;
      break;
    }
    case "remote_support_ended": {
      // Session summary (CF-237); numbers and flags arrive as JSON values.
      const raw = (m.attributes ?? {}) as Record<string, unknown>;
      const seconds = Number(raw.duration_seconds) || 0;
      const minutes = Math.floor(seconds / 60);
      const parts = [
        t(
          `inbox.events.remote_support_mode_${a.mode === "control" ? "control" : "view"}`,
        ),
      ];
      if (seconds > 0)
        parts.push(
          t("inbox.events.remote_support_duration", {
            minutes,
            seconds: String(seconds % 60).padStart(2, "0"),
          }),
        );
      if (raw.control_consent === true)
        parts.push(t("inbox.events.remote_support_control_given"));
      if (raw.recording === true)
        parts.push(t("inbox.events.remote_support_recorded"));
      text = `${t(
        raw.view_consent === true
          ? "inbox.events.remote_support_ended"
          : "inbox.events.remote_support_not_started",
      )} (${parts.join(", ")})`;
      break;
    }
    case "merged_from":
    case "merged_into":
    case "split_to":
    case "split_from":
    case "unlinked":
      text = t(`inbox.events.${a.event}`, { actor, number: a.to ?? "" });
      break;
    case "linked":
      text = t("inbox.events.linked", {
        actor,
        number: a.to ?? "",
        relation: t(`inbox.links.relation.${a.relation ?? "related"}`),
      });
      break;
    case "side_conversation_started":
    case "side_conversation_sent":
    case "side_conversation_reply":
      text = t(`inbox.events.${a.event}`, {
        actor,
        to: a.to ?? "",
        subject: a.subject ?? "",
      });
      break;
    case "appointment_booked":
    case "appointment_rescheduled":
    case "appointment_confirmed":
    case "appointment_completed":
    case "appointment_no_show":
    case "appointment_cancelled":
      text = t(APPOINTMENT_EVENT_KEYS[a.event], {
        actor,
        type: a.type ?? "",
        to: a.to ?? "",
        from: a.from ?? "",
      });
      break;
    default:
      text = a.event ?? "";
  }
  return (
    <div className="text-muted-foreground flex items-center justify-center gap-2 py-2 text-[11px]">
      <span className="bg-border h-px w-6" />
      <span>{text}</span>
      <span className="tabular-nums opacity-70">
        {new Date(m.created_at).toLocaleTimeString([], {
          hour: "2-digit",
          minute: "2-digit",
        })}
      </span>
      <span className="bg-border h-px w-6" />
    </div>
  );
}
