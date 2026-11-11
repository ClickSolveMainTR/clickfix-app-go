"use client";

import {
  ThreadListItemPrimitive,
  ThreadListPrimitive,
  useAui,
  useAuiState,
} from "@assistant-ui/react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Bot,
  History,
  LifeBuoy,
  Maximize2,
  MessagesSquare,
  PanelRightClose,
  PanelRightOpen,
  Plus,
  X,
} from "lucide-react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { useEffect, useRef, useState } from "react";

import { Button } from "@/components/ui/button";
import { ScrollArea } from "@/components/ui/scroll-area";
import { SheetDescription, SheetTitle } from "@/components/ui/sheet";
import { Textarea } from "@/components/ui/textarea";
import { routes } from "@/config/routes";
import { guidesService } from "@/features/guides/services/guides.service";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { errorKey } from "../lib/errors";
import { openSupportChat } from "../lib/support-widget";
import { bumpUsage, runSettled } from "../lib/usage";
import { useAssistantSurface } from "../runtime/surface";
import { assistantService } from "../services/assistant.service";

import { useOptionalAssistant } from "./assistant-provider";
import { AssistantThread } from "./assistant-thread";
import { PageStateCard } from "./page-state-card";
import { useSupportWidget } from "./support-widget";

const keys = {
  status: (slug: string) => ["assistant", slug, "status"] as const,
};

export type AssistantPanelVariant = "sheet" | "docked" | "page";

/**
 * The ClickFix Asistan panel: header (history, new chat, dock / full page),
 * the thread, and the footer with hand-over to ClickFix support and usage.
 * The same panel renders in the sheet, the docked column and the full page;
 * the page variant has no header (the page header and the list beside it
 * take over).
 */
export function AssistantPanel({
  variant,
  onClose,
}: {
  variant: AssistantPanelVariant;
  onClose?: () => void;
}) {
  const { t, locale } = useLocale();
  const { slug, ensureThread } = useAssistantSurface();
  const pathname = usePathname() || "/";
  const assistant = useOptionalAssistant();
  const aui = useAui();
  const sendPrompt = (prompt: string) => aui.thread().append(prompt);
  const [showHistory, setShowHistory] = useState(false);
  const [escalating, setEscalating] = useState(false);
  const [note, setNote] = useState("");
  const [notice, setNotice] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const supportWidget = useSupportWidget(slug, true);
  const isEmpty = useAuiState((s) => s.thread.messages.length === 0);
  const isRunning = useAuiState((s) => s.thread.isRunning);

  const qc = useQueryClient();
  const status = useQuery({
    queryKey: keys.status(slug),
    queryFn: () => assistantService.status(slug),
    enabled: Boolean(slug),
  });
  // "Today N/limit" moves with every reply (CF-207, #150): bump the cached
  // count as soon as a run settles, then fetch the exact figure.
  const wasRunning = useRef(isRunning);
  useEffect(() => {
    if (runSettled(wasRunning.current, isRunning)) {
      qc.setQueryData(keys.status(slug), bumpUsage);
      void qc.invalidateQueries({ queryKey: keys.status(slug) });
    }
    wasRunning.current = isRunning;
  }, [isRunning, qc, slug]);
  const pageGuides = useQuery({
    queryKey: ["guides", slug, "route", pathname, locale],
    queryFn: () => guidesService.forRoute(slug, pathname, locale),
    enabled: Boolean(slug) && isEmpty,
  });
  const pageState = useQuery({
    queryKey: ["assistant", slug, "page-state", pathname, locale],
    queryFn: () => assistantService.pageState(slug, pathname, locale),
    enabled: Boolean(slug) && isEmpty,
    staleTime: 30_000,
  });
  const pageTitle = pageGuides.data?.items.find(
    (g) => g.kind === "page",
  )?.title;
  const disabled =
    Boolean(status.data && !status.data.enabled) || status.isError;

  const escalate = useMutation({
    mutationFn: async () =>
      assistantService.escalate(
        slug,
        await ensureThread(),
        note.trim(),
        locale,
      ),
    onMutate: () => setError(null),
    onSuccess: (out) => {
      setEscalating(false);
      setNote("");
      setNotice(t("assistant.escalate.done", { ref: out.ticket_ref }));
    },
    onError: (err) => setError(errorKey(err)),
  });

  const starters = [
    pageTitle
      ? t("assistant.starters.context", { title: pageTitle })
      : t("assistant.starters.page"),
    t("assistant.starters.setup"),
    t("assistant.starters.invite"),
    t("assistant.starters.problem"),
  ];

  return (
    <>
      {variant !== "page" ? (
        <div className="flex items-center gap-2 border-b px-4 py-3 pr-12">
          <span className="bg-primary/10 text-primary flex size-8 items-center justify-center rounded-lg">
            <Bot className="size-4" />
          </span>
          <div className="min-w-0 flex-1">
            {variant === "sheet" ? (
              <>
                <SheetTitle className="text-base">
                  {t("assistant.title")}
                </SheetTitle>
                <SheetDescription className="truncate text-xs">
                  {t("assistant.subtitle")}
                </SheetDescription>
              </>
            ) : (
              <>
                <p className="text-base font-semibold">
                  {t("assistant.title")}
                </p>
                <p className="text-muted-foreground truncate text-xs">
                  {t("assistant.subtitle")}
                </p>
              </>
            )}
          </div>
          <Button
            variant="ghost"
            size="icon"
            className="size-8"
            aria-label={
              showHistory ? t("assistant.history_back") : t("assistant.history")
            }
            aria-pressed={showHistory}
            onClick={() => setShowHistory((s) => !s)}
          >
            <History className="size-4" />
          </Button>
          <ThreadListPrimitive.New asChild>
            <Button
              variant="ghost"
              size="icon"
              className="size-8"
              aria-label={t("assistant.new_chat")}
              onClick={() => {
                setShowHistory(false);
                setError(null);
                setNotice(null);
              }}
            >
              <Plus className="size-4" />
            </Button>
          </ThreadListPrimitive.New>
          {assistant ? (
            <Button
              variant="ghost"
              size="icon"
              className="hidden size-8 lg:inline-flex"
              aria-label={
                variant === "docked"
                  ? t("assistant.mode.undock")
                  : t("assistant.mode.dock")
              }
              title={
                variant === "docked"
                  ? t("assistant.mode.undock")
                  : t("assistant.mode.dock")
              }
              onClick={() =>
                assistant.setMode(variant === "docked" ? "drawer" : "docked")
              }
            >
              {variant === "docked" ? (
                <PanelRightClose className="size-4" />
              ) : (
                <PanelRightOpen className="size-4" />
              )}
            </Button>
          ) : null}
          <Button
            asChild
            variant="ghost"
            size="icon"
            className="size-8"
            aria-label={t("assistant.mode.page")}
            title={t("assistant.mode.page")}
          >
            <Link href={routes.cms.assistant} onClick={onClose}>
              <Maximize2 className="size-4" />
            </Link>
          </Button>
          {variant === "docked" && onClose ? (
            <Button
              variant="ghost"
              size="icon"
              className="absolute top-3 right-3 size-8"
              aria-label={t("assistant.close")}
              onClick={onClose}
            >
              <X className="size-4" />
            </Button>
          ) : null}
        </div>
      ) : null}

      {showHistory && variant !== "page" ? (
        <ThreadHistory onPick={() => setShowHistory(false)} />
      ) : (
        <div className="min-h-0 flex-1">
          <AssistantThread
            disabled={disabled}
            starters={starters}
            welcome={
              <>
                <div className="bg-card rounded-2xl border p-5 text-sm shadow-xs">
                  <p className="font-medium">{t("assistant.welcome_title")}</p>
                  <p className="text-muted-foreground mt-1">
                    {t("assistant.welcome_body")}
                  </p>
                </div>
                {status.isError ? (
                  <QueryFailure onRetry={() => void status.refetch()} />
                ) : null}
                {pageState.data ? (
                  <PageStateCard
                    state={pageState.data}
                    disabled={disabled || isRunning}
                    onAsk={sendPrompt}
                  />
                ) : null}
              </>
            }
          />
        </div>
      )}

      <div className="space-y-2 border-t px-3 py-2">
        {disabled ? (
          <p className="text-muted-foreground text-xs">
            {t(`assistant.status.${status.data?.reason ?? "disabled"}`)}
          </p>
        ) : null}
        {error ? (
          <p role="alert" className="text-destructive text-xs">
            {t(error)}
          </p>
        ) : null}
        {notice ? (
          <p role="status" className="text-muted-foreground text-xs">
            {notice}
          </p>
        ) : null}
        {escalating && status.data?.escalation_available !== false ? (
          <div className="space-y-2 rounded-lg border p-2">
            <p className="text-xs">{t("assistant.escalate.hint")}</p>
            <Textarea
              autoFocus
              aria-label={t("assistant.escalate.placeholder")}
              rows={2}
              maxLength={2000}
              value={note}
              onChange={(e) => setNote(e.target.value)}
              placeholder={t("assistant.escalate.placeholder")}
            />
            <div className="flex justify-end gap-2">
              <Button
                size="sm"
                variant="ghost"
                onClick={() => setEscalating(false)}
              >
                {t("common.cancel")}
              </Button>
              <Button
                size="sm"
                disabled={escalate.isPending}
                onClick={() => escalate.mutate()}
              >
                {t("assistant.escalate.send")}
              </Button>
            </div>
          </div>
        ) : null}
        <div className="text-muted-foreground flex items-center justify-between text-[11px]">
          {/* No support organization/inbox: escalation would answer 409
              ASSISTANT_SUPPORT_UNAVAILABLE, so the action is hidden. */}
          <span className="inline-flex items-center gap-3">
            {status.data?.escalation_available === false ? null : (
              <button
                type="button"
                className="hover:text-foreground inline-flex items-center gap-1"
                onClick={() => setEscalating((v) => !v)}
              >
                <LifeBuoy className="size-3.5" />
                {t("assistant.escalate.button")}
              </button>
            )}
            {/* ClickFix's support widget (no launcher in the panel): a live
                conversation with the support team. */}
            {supportWidget.data?.available ? (
              <button
                type="button"
                className="hover:text-foreground inline-flex items-center gap-1"
                onClick={() => {
                  if (openSupportChat()) onClose?.();
                }}
              >
                <MessagesSquare className="size-3.5" />
                {t("assistant.support_chat")}
              </button>
            ) : null}
          </span>
          {status.data ? (
            <span>
              {t("assistant.usage", {
                used: status.data.used_today,
                limit: status.data.daily_limit,
              })}
            </span>
          ) : null}
        </div>
      </div>
    </>
  );
}

/** Past chats of the member (RemoteThreadListAdapter over GET threads). */
export function ThreadHistory({ onPick }: { onPick?: () => void }) {
  const { t } = useLocale();
  const loading = useAuiState((s) => s.threads.isLoading);
  const count = useAuiState((s) => s.threads.threadIds.length);
  return (
    <ScrollArea className="min-h-0 flex-1">
      {loading ? (
        <p role="status" className="text-muted-foreground p-6 text-sm">
          {t("common.loading")}
        </p>
      ) : null}
      <ThreadListPrimitive.Root asChild>
        <ul className="divide-y">
          <ThreadListPrimitive.Items
            components={{
              ThreadListItem: () => <ThreadHistoryItem onPick={onPick} />,
            }}
          />
          {!loading && count === 0 ? (
            <li className="text-muted-foreground px-4 py-6 text-sm">
              {t("assistant.no_history")}
            </li>
          ) : null}
        </ul>
      </ThreadListPrimitive.Root>
    </ScrollArea>
  );
}

function ThreadHistoryItem({ onPick }: { onPick?: () => void }) {
  const { t } = useLocale();
  const title = useAuiState((s) => s.threadListItem.title);
  const isMain = useAuiState(
    (s) => s.threads.mainThreadId === s.threadListItem.id,
  );
  return (
    <ThreadListItemPrimitive.Root asChild>
      <li>
        <ThreadListItemPrimitive.Trigger asChild>
          <button
            type="button"
            className={cn(
              "hover:bg-muted focus-visible:ring-ring w-full px-4 py-3 text-left text-sm focus-visible:ring-2 focus-visible:outline-none focus-visible:ring-inset",
              isMain && "bg-muted",
            )}
            aria-current={isMain ? "true" : undefined}
            onClick={onPick}
          >
            <span className="block truncate font-medium">
              {title || t("assistant.untitled")}
            </span>
          </button>
        </ThreadListItemPrimitive.Trigger>
      </li>
    </ThreadListItemPrimitive.Root>
  );
}

function QueryFailure({ onRetry }: { onRetry: () => void }) {
  const { t } = useLocale();
  return (
    <div
      role="alert"
      className="bg-destructive/5 text-destructive border-destructive/20 space-y-2 rounded-xl border p-3 text-sm"
    >
      <p>{t("assistant.errors.generic")}</p>
      <Button variant="outline" size="sm" onClick={onRetry}>
        {t("common.retry")}
      </Button>
    </div>
  );
}
