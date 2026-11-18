"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import {
  AlarmClock,
  ArrowLeft,
  BookUser,
  Bookmark,
  CheckCircle2,
  Flag,
  Hash,
  Inbox,
  Keyboard,
  Mail,
  MessageSquareReply,
  RotateCcw,
  Search,
  StickyNote,
  UserRound,
  UserRoundCheck,
  X,
  Zap,
} from "lucide-react";

import { Button } from "@/components/ui/button";
import { Sheet, SheetContent, SheetTitle } from "@/components/ui/sheet";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { routes } from "@/config/routes";
import { useIsXl } from "@/hooks/use-mobile";
import { useApplyMacro, useMacros } from "@/features/macros/hooks/use-macros";
import {
  usePaletteActions,
  type PaletteAction,
} from "@/features/search-engine/lib/actions";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import {
  useConversation,
  useConversationList,
  useConversationPresence,
  useConversationViews,
  useInboxCounts,
  useInboxLive,
  useInboxScope,
  useMarkRead,
  useMessages,
  useReadState,
  useRetryMessage,
} from "../hooks/use-inbox";
import { isTyping } from "../lib/shortcuts";
import type {
  InboxFilters,
  InboxStatusTab,
  InboxView,
  OptimisticMessage,
} from "../types";
import { Composer, type ComposerHandle } from "./composer";
import { FilterBar } from "./filter-bar";
import { ContextPanel } from "./context-panel";
import {
  ConversationHeader,
  type HeaderHandle,
  type Picker,
} from "./conversation-header";
import { ConversationList } from "./conversation-list";
import { MessageTimeline } from "./message-timeline";
import { BulkBar } from "./bulk-bar";
import { SplitDialog } from "./split-dialog";
import { EditMessageDialog } from "./edit-message-dialog";
import { ShortcutsDialog } from "./shortcuts-dialog";
import { ViewersBar } from "./viewers-bar";

const VIEWS: InboxView[] = [
  "all",
  "mine",
  "teams",
  "unassigned",
  "mentions",
  "sla_soon",
  "breached",
];
const STATUSES: InboxStatusTab[] = ["open", "snoozed", "resolved", "all"];

const INBOX_ACTION_EVENT = "clickfix:inbox-action";

/** Asks the open inbox to run a conversation action (palette, CF-50). */
function inboxAction(action: string) {
  window.dispatchEvent(new CustomEvent(INBOX_ACTION_EVENT, { detail: action }));
}

function asView(v: string | null): InboxView {
  return VIEWS.includes(v as InboxView) ? (v as InboxView) : "all";
}
function asStatus(v: string | null): InboxStatusTab {
  return STATUSES.includes(v as InboxStatusTab)
    ? (v as InboxStatusTab)
    : "open";
}

/**
 * The unified inbox: conversation list · conversation · context.
 * State lives in the URL (?view, ?status, ?c) so views are linkable and
 * the sidebar items deep-link. Keyboard first (press ? for the map).
 */
export function InboxPage() {
  const { t } = useLocale();
  const router = useRouter();
  const pathname = usePathname();
  const params = useSearchParams();
  const scope = useInboxScope();
  const [splitting, setSplitting] = useState<OptimisticMessage | null>(null);
  const [editing, setEditing] = useState<OptimisticMessage | null>(null);

  const view = asView(params.get("view"));
  const status = asStatus(params.get("status"));
  const activeUuid = params.get("c");
  // Cmd+K contact / company hits land here (?contact, ?company).
  const contact = params.get("contact") ?? undefined;
  const company = params.get("company") ?? undefined;
  // Advanced filters (?pr, ?ch, ?tg) and the saved view (?sv), CF-47.
  const prParam = params.get("pr");
  const chParam = params.get("ch");
  const tgParam = params.get("tg");
  const brParam = params.get("br");
  // CF-195: contact identity (?id=verified|anonymous|self_named).
  const idParam = asIdentity(params.get("id"));
  const savedView = params.get("sv");
  const savedViews = useConversationViews();
  const saved = savedViews.data?.find((v) => v.uuid === savedView)?.filters;
  const [q, setQ] = useState("");
  const [debouncedQ, setDebouncedQ] = useState("");
  const [unread, setUnread] = useState(false);
  const [contextOpen, setContextOpen] = useState(true);
  const [sheetOpen, setSheetOpen] = useState(false);
  const isXl = useIsXl();
  const [helpOpen, setHelpOpen] = useState(false);
  // Bulk selection (CF-51): X toggles the open conversation, the row
  // checkboxes toggle any; Esc clears.
  const [selected, setSelected] = useState<ReadonlySet<string>>(new Set());
  const toggleSelected = useCallback((uuid: string) => {
    setSelected((cur) => {
      const next = new Set(cur);
      if (next.has(uuid)) next.delete(uuid);
      else next.add(uuid);
      return next;
    });
  }, []);
  const searchRef = useRef<HTMLInputElement>(null);
  const composerRef = useRef<ComposerHandle>(null);
  const headerRef = useRef<HeaderHandle>(null);

  useEffect(() => {
    const id = setTimeout(() => setDebouncedQ(q.trim()), 250);
    return () => clearTimeout(id);
  }, [q]);

  const filters = useMemo<InboxFilters>(
    () => ({
      view,
      status,
      q: debouncedQ,
      unread,
      sort: "-last_message_at",
      contact,
      company,
      priorities: prParam ? prParam.split(",") : undefined,
      channelTypes: chParam ? chParam.split(",") : undefined,
      tags: tgParam ? tgParam.split(",") : undefined,
      brand: brParam ?? undefined,
      identity: idParam,
      saved,
    }),
    [
      view,
      status,
      debouncedQ,
      unread,
      contact,
      company,
      prParam,
      chParam,
      tgParam,
      brParam,
      idParam,
      saved,
    ],
  );

  const setParams = useCallback(
    (patch: Record<string, string | null>) => {
      const next = new URLSearchParams(params.toString());
      for (const [k, v] of Object.entries(patch)) {
        if (v == null) next.delete(k);
        else next.set(k, v);
      }
      router.replace(`${pathname}?${next.toString()}`, { scroll: false });
    },
    [params, pathname, router],
  );

  const list = useConversationList(filters);
  const counts = useInboxCounts();
  const detail = useConversation(activeUuid);
  const messages = useMessages(activeUuid);
  const retry = useRetryMessage(activeUuid ?? "");
  const markRead = useMarkRead(activeUuid);
  const readState = useReadState();
  const [typing, setTyping] = useState(false);
  const viewers = useConversationPresence(activeUuid, typing);

  useInboxLive(activeUuid, markRead);

  // Opening a conversation marks it read.
  useEffect(() => {
    if (activeUuid && detail.data && detail.data.unread_count > 0) markRead();
  }, [activeUuid, detail.data, markRead]);

  const select = useCallback(
    (uuid: string | null) => setParams({ c: uuid }),
    [setParams],
  );

  // Marking unread (CF-247) closes the conversation when it is the open
  // one: an open conversation is marked read again.
  const { mutate: mutateUnread } = readState.markUnread;
  const { mutate: mutateRead } = readState.markRead;
  const setReadState = useCallback(
    (uuid: string, unread: boolean) => {
      if (!unread) {
        mutateRead(uuid);
        return;
      }
      if (uuid === activeUuid) select(null);
      mutateUnread(uuid);
    },
    [activeUuid, select, mutateRead, mutateUnread],
  );
  const markUnreadActive = useCallback(() => {
    if (activeUuid) setReadState(activeUuid, true);
  }, [activeUuid, setReadState]);

  const move = useCallback(
    (step: 1 | -1) => {
      const items = list.items;
      if (items.length === 0) return;
      const i = items.findIndex((c) => c.uuid === activeUuid);
      const next =
        items[Math.max(0, Math.min(items.length - 1, i < 0 ? 0 : i + step))];
      if (next) select(next.uuid);
    },
    [list.items, activeUuid, select],
  );

  // Cmd+K actions for this screen and the open conversation (CF-50).
  const openPicker = useCallback((p: Picker) => headerRef.current?.open(p), []);
  const toggleResolve = useCallback(
    () => headerRef.current?.toggleResolve(),
    [],
  );
  const takeIt = useCallback(() => headerRef.current?.takeIt(), []);
  const focusComposer = useCallback(
    (m: "reply" | "note") => composerRef.current?.focus(m),
    [],
  );
  const conversation = detail.data;
  const macros = useMacros();
  const { mutate: applyMacro } = useApplyMacro(activeUuid ?? "");
  const paletteActions = useMemo<PaletteAction[]>(() => {
    const group = t("inbox.palette.group_views");
    const out: PaletteAction[] = VIEWS.map((v) => ({
      id: `inbox.view.${v}`,
      label: t("inbox.palette.go_view", { view: t(`inbox.views.${v}`) }),
      group,
      icon: Inbox,
      keywords: "görünüm view",
      run: () => setParams({ view: v, c: null }),
    }));
    out.push({
      id: "inbox.shortcuts",
      label: t("inbox.shortcuts.title"),
      group,
      icon: Keyboard,
      run: () => setHelpOpen(true),
    });
    for (const sv of savedViews.data ?? []) {
      out.push({
        id: `inbox.saved.${sv.uuid}`,
        label: t("inbox.palette.go_view", { view: sv.name }),
        group,
        icon: Bookmark,
        keywords: "kayıtlı görünüm saved view",
        run: () => setParams({ sv: sv.uuid, c: null }),
      });
    }
    if (!conversation) return out;
    const cg = t("inbox.palette.group_conversation", {
      number: conversation.number,
    });
    const resolved =
      conversation.status === "resolved" || conversation.status === "closed";
    const add = (a: Omit<PaletteAction, "group">) =>
      out.unshift({ ...a, group: cg });
    if (scope.canManage) {
      add({
        id: "inbox.snooze",
        label: t("inbox.palette.snooze"),
        icon: AlarmClock,
        run: () => inboxAction("snooze"),
      });
      add({
        id: "inbox.tag",
        label: t("inbox.palette.tag"),
        icon: Hash,
        run: () => inboxAction("tag"),
      });
      add({
        id: "inbox.priority",
        label: t("inbox.palette.priority"),
        icon: Flag,
        run: () => inboxAction("priority"),
      });
    }
    if (scope.canAssign) {
      add({
        id: "inbox.assign",
        label: t("inbox.palette.assign"),
        icon: UserRound,
        run: () => inboxAction("assign"),
      });
      add({
        id: "inbox.take",
        label: t("inbox.shortcuts.take"),
        icon: UserRoundCheck,
        run: () => inboxAction("take"),
      });
    }
    if (scope.canNote || scope.canReply) {
      add({
        id: "inbox.note",
        label: t("inbox.shortcuts.note"),
        icon: StickyNote,
        run: () => inboxAction("note"),
      });
    }
    if (scope.canReply) {
      add({
        id: "inbox.reply",
        label: t("inbox.shortcuts.reply"),
        icon: MessageSquareReply,
        run: () => inboxAction("reply"),
      });
    }
    if (scope.canManage) {
      add({
        id: "inbox.resolve",
        label: resolved ? t("inbox.header.reopen") : t("inbox.header.resolve"),
        icon: resolved ? RotateCcw : CheckCircle2,
        keywords: "çöz kapat resolve close reopen",
        run: () => inboxAction("resolve"),
      });
    }
    for (const m of macros.data ?? []) {
      out.push({
        id: `inbox.macro.${m.uuid}`,
        label: t("inbox.palette.macro", { name: m.name }),
        group: cg,
        icon: Zap,
        keywords: "makro macro",
        run: () => applyMacro(m.uuid),
      });
    }
    out.push({
      id: "inbox.unread",
      label: t("inbox.read_state.mark_unread"),
      group: cg,
      icon: Mail,
      keywords: "okunmadı unread",
      run: markUnreadActive,
    });
    out.push({
      id: "inbox.contact",
      label: t("inbox.palette.open_contact", {
        name: conversation.contact.name || conversation.contact.email || "",
      }),
      group: cg,
      icon: BookUser,
      run: () =>
        router.push(routes.cms.contacts.detail(conversation.contact.uuid)),
    });
    return out;
  }, [
    conversation,
    savedViews.data,
    macros.data,
    applyMacro,
    markUnreadActive,
    scope.canManage,
    scope.canAssign,
    scope.canNote,
    scope.canReply,
    setParams,
    router,
    t,
  ]);
  usePaletteActions("inbox", paletteActions);

  // Runs the conversation actions the palette dispatches (inboxAction).
  useEffect(() => {
    const onAction = (e: Event) => {
      const action = (e as CustomEvent<string>).detail;
      switch (action) {
        case "assign":
        case "tag":
        case "priority":
        case "snooze":
          openPicker(action);
          return;
        case "take":
          takeIt();
          return;
        case "resolve":
          toggleResolve();
          return;
        case "reply":
        case "note":
          focusComposer(action);
          return;
      }
    };
    window.addEventListener(INBOX_ACTION_EVENT, onAction);
    return () => window.removeEventListener(INBOX_ACTION_EVENT, onAction);
  }, [openPicker, takeIt, toggleResolve, focusComposer]);

  // Global shortcuts (off while typing).
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.metaKey || e.ctrlKey || e.altKey) return;
      if (isTyping(e)) return;
      const k = e.key;
      const conv = Boolean(activeUuid && detail.data);
      const handled = (() => {
        switch (k) {
          case "j":
          case "ArrowDown":
            move(1);
            return true;
          case "k":
          case "ArrowUp":
            move(-1);
            return true;
          case "/":
            searchRef.current?.focus();
            return true;
          case "?":
            setHelpOpen(true);
            return true;
          case "1":
          case "2":
          case "3":
          case "4":
            setParams({ view: VIEWS[Number(k) - 1] ?? "all", c: null });
            return true;
          case "x":
            if (activeUuid) toggleSelected(activeUuid);
            return Boolean(activeUuid);
          case "Escape":
            if (selected.size > 0) {
              setSelected(new Set());
              return true;
            }
            if (activeUuid) select(null);
            return Boolean(activeUuid);
        }
        if (!conv) return false;
        switch (k) {
          case "r":
            composerRef.current?.focus("reply");
            return true;
          case "n":
            composerRef.current?.focus("note");
            return true;
          case "e":
            headerRef.current?.toggleResolve();
            return true;
          case "a":
            headerRef.current?.open("assign");
            return true;
          case "i":
            headerRef.current?.takeIt();
            return true;
          case "#":
            headerRef.current?.open("tag");
            return true;
          case "p":
            headerRef.current?.open("priority");
            return true;
          case "s":
            headerRef.current?.open("snooze");
            return true;
          case "m":
            headerRef.current?.open("macro");
            return true;
          case "g":
            composerRef.current?.copilot("suggest_reply");
            return true;
          case "u":
            composerRef.current?.copilot("summarize");
            return true;
          case "q":
            composerRef.current?.copilot("ask");
            return true;
          case "U":
            markUnreadActive();
            return true;
        }
        return false;
      })();
      if (handled) e.preventDefault();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [
    activeUuid,
    detail.data,
    move,
    select,
    setParams,
    selected,
    toggleSelected,
    markUnreadActive,
  ]);

  if (!scope.org) {
    return (
      <p className="text-muted-foreground p-8 text-sm">
        {t("inbox.no_organization")}
      </p>
    );
  }
  if (!scope.canRead) {
    return (
      <p className="text-muted-foreground p-8 text-sm">
        {t("inbox.no_permission")}
      </p>
    );
  }

  const viewCount = (v: InboxView) =>
    v === "mine"
      ? counts.data?.mine
      : v === "unassigned"
        ? counts.data?.unassigned
        : v === "all"
          ? counts.data?.open
          : undefined;

  return (
    <div
      className="bg-background -mx-4 -mb-6 flex h-[calc(100svh-4.5rem)] min-h-[480px] overflow-hidden border-t md:-mx-6"
      data-inbox
    >
      {/* Pane 1: list */}
      <section
        className={cn(
          "flex w-full shrink-0 flex-col border-r md:w-[340px] xl:w-[380px]",
          activeUuid && "hidden md:flex",
        )}
        aria-label={t("inbox.list.label")}
      >
        <div className="flex flex-col gap-2 border-b p-3">
          <div
            className="flex items-center gap-1 overflow-x-auto"
            data-guide="inbox-views"
          >
            {VIEWS.map((v, i) => (
              <button
                type="button"
                key={v}
                onClick={() => setParams({ view: v, c: null })}
                className={cn(
                  "flex shrink-0 items-center gap-1.5 rounded-md px-2.5 py-1 text-xs font-medium transition-colors",
                  view === v
                    ? "bg-foreground text-background"
                    : "text-muted-foreground hover:bg-muted hover:text-foreground",
                )}
                title={`${t(`inbox.views.${v}`)} (${i + 1})`}
              >
                {t(`inbox.views.${v}`)}
                {viewCount(v) != null && viewCount(v)! > 0 && (
                  <span className="tabular-nums opacity-70">
                    {viewCount(v)}
                  </span>
                )}
              </button>
            ))}
          </div>
          <div className="relative">
            <Search className="text-muted-foreground absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2" />
            <input
              ref={searchRef}
              value={q}
              onChange={(e) => setQ(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Escape") {
                  setQ("");
                  e.currentTarget.blur();
                }
              }}
              placeholder={t("inbox.list.search")}
              aria-label={t("inbox.list.search")}
              className="bg-muted/50 focus:bg-background focus:ring-ring/40 h-8 w-full rounded-md border pr-7 pl-8 text-sm outline-none focus:ring-2"
            />
            {q && (
              <button
                type="button"
                className="absolute top-1/2 right-2 -translate-y-1/2"
                onClick={() => setQ("")}
                aria-label={t("inbox.list.clear_search")}
              >
                <X className="text-muted-foreground size-3.5" />
              </button>
            )}
          </div>
          {(contact || company) && (
            <div className="bg-signal-soft text-signal-text flex items-center justify-between gap-2 rounded-md px-2 py-1 text-xs font-medium">
              <span className="truncate">
                {t(
                  contact
                    ? "inbox.list.filter_contact"
                    : "inbox.list.filter_company",
                )}
              </span>
              <button
                type="button"
                onClick={() => setParams({ contact: null, company: null })}
                aria-label={t("inbox.list.clear_filter")}
                title={t("inbox.list.clear_filter")}
              >
                <X className="size-3.5" />
              </button>
            </div>
          )}
          <Tabs
            value={status}
            onValueChange={(v) => setParams({ status: v, c: null })}
          >
            <TabsList className="grid h-8 w-full grid-cols-4">
              {STATUSES.map((s) => (
                <TabsTrigger
                  key={s}
                  value={s}
                  className="h-7 min-w-0 px-1.5 text-xs"
                >
                  <span className="truncate">
                    {t(`inbox.status_tabs.${s}`)}
                  </span>
                </TabsTrigger>
              ))}
            </TabsList>
          </Tabs>
          <div className="flex min-w-0 items-center justify-between gap-2">
            <button
              type="button"
              onClick={() => setUnread((u) => !u)}
              aria-pressed={unread}
              className={cn(
                "rounded-md px-2 py-1 text-xs font-medium",
                unread
                  ? "bg-signal-soft text-signal-text"
                  : "text-muted-foreground hover:text-foreground",
              )}
            >
              {t("inbox.list.unread_only")}
            </button>
            <FilterBar
              filters={filters}
              savedView={savedView}
              setParams={setParams}
            />
          </div>
        </div>
        {selected.size > 0 && (
          <BulkBar
            selected={[...selected]}
            onClear={() => setSelected(new Set())}
          />
        )}
        <div className="min-h-0 flex-1" data-guide="inbox-list">
          <ConversationList
            items={list.items}
            activeUuid={activeUuid}
            loading={list.isLoading}
            hasMore={Boolean(list.hasNextPage)}
            loadingMore={list.isFetchingNextPage}
            onLoadMore={() => void list.fetchNextPage()}
            onSelect={select}
            selected={selected}
            onToggle={toggleSelected}
            onReadState={setReadState}
          />
        </div>
        <div className="text-muted-foreground flex items-center justify-between border-t px-3 py-1.5 text-[11px]">
          <span className="tabular-nums">
            {t("inbox.list.total", { count: list.total })}
          </span>
          <button
            type="button"
            onClick={() => setHelpOpen(true)}
            className="hover:text-foreground flex items-center gap-1"
          >
            <Keyboard className="size-3.5" />
            {t("inbox.shortcuts.hint")}
          </button>
        </div>
      </section>

      {/* Pane 2: conversation */}
      <section
        className={cn(
          "flex min-w-0 flex-1 flex-col",
          !activeUuid && "hidden md:flex",
        )}
        aria-label={t("inbox.conversation.label")}
      >
        {activeUuid && detail.data ? (
          <>
            <div className="flex items-center md:hidden">
              <Button
                variant="ghost"
                size="sm"
                className="m-1"
                onClick={() => select(null)}
              >
                <ArrowLeft className="size-4" />
                {t("inbox.conversation.back")}
              </Button>
            </div>
            <ConversationHeader
              ref={headerRef}
              conversation={detail.data}
              canManage={scope.canManage}
              canAssign={scope.canAssign}
              contextOpen={isXl ? contextOpen : sheetOpen}
              onToggleContext={() =>
                isXl ? setContextOpen((o) => !o) : setSheetOpen((o) => !o)
              }
              onMarkUnread={markUnreadActive}
            />
            <MessageTimeline
              conversationUuid={activeUuid}
              items={messages.items}
              loading={messages.isLoading}
              hasOlder={Boolean(messages.hasNextPage)}
              loadingOlder={messages.isFetchingNextPage}
              onLoadOlder={() => void messages.fetchNextPage()}
              onRetry={(m) => retry.mutate(m)}
              onSplit={scope.canManage ? setSplitting : undefined}
              onEdit={scope.canReply ? setEditing : undefined}
            />
            {editing && (
              <EditMessageDialog
                conversation={detail.data}
                message={editing}
                onClose={() => setEditing(null)}
              />
            )}
            <ViewersBar viewers={viewers} />
            {splitting && (
              <SplitDialog
                conversation={detail.data}
                message={splitting}
                onClose={() => setSplitting(null)}
                onSplit={(uuid) => {
                  setSplitting(null);
                  select(uuid);
                }}
              />
            )}
            <div data-guide="inbox-composer" className="shrink-0">
              <Composer
                key={activeUuid}
                ref={composerRef}
                conversation={detail.data}
                canReply={scope.canReply}
                canNote={scope.canNote}
                onTypingChange={setTyping}
              />
            </div>
          </>
        ) : activeUuid && detail.isError ? (
          <EmptyPane title={t("inbox.conversation.not_found")} hint="" />
        ) : activeUuid ? (
          <div className="flex-1" aria-busy />
        ) : (
          <EmptyPane
            title={t("inbox.conversation.pick_title")}
            hint={t("inbox.conversation.pick_hint")}
          />
        )}
      </section>

      {/* Pane 3: context */}
      {activeUuid && detail.data && isXl && contextOpen && (
        <section className="bg-muted/20 w-[320px] shrink-0 border-l">
          <ContextPanel
            conversation={detail.data}
            onOpen={(uuid) => select(uuid)}
          />
        </section>
      )}
      {/* Below xl there is no room for a third column, so it slides in. */}
      <Sheet
        open={!isXl && sheetOpen && Boolean(activeUuid && detail.data)}
        onOpenChange={setSheetOpen}
      >
        <SheetContent side="right" className="w-[92vw] p-0 sm:max-w-[360px]">
          <SheetTitle className="sr-only">
            {t("inbox.context.label")}
          </SheetTitle>
          {detail.data && (
            <ContextPanel
              conversation={detail.data}
              onOpen={(uuid) => {
                setSheetOpen(false);
                select(uuid);
              }}
            />
          )}
        </SheetContent>
      </Sheet>

      <ShortcutsDialog open={helpOpen} onOpenChange={setHelpOpen} />
    </div>
  );
}

function EmptyPane({ title, hint }: { title: string; hint: string }) {
  return (
    <div className="flex flex-1 flex-col items-center justify-center gap-2 p-8 text-center">
      <div className="bg-signal-soft text-signal-text mb-1 flex size-10 items-center justify-center rounded-full">
        <Keyboard className="size-5" />
      </div>
      <p className="text-sm font-medium">{title}</p>
      {hint && <p className="text-muted-foreground max-w-xs text-xs">{hint}</p>}
    </div>
  );
}

const IDENTITY_STATES = ["verified", "anonymous", "self_named"] as const;

function asIdentity(
  v: string | null,
): (typeof IDENTITY_STATES)[number] | undefined {
  return IDENTITY_STATES.find((s) => s === v);
}
