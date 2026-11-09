"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Bot,
  Copy,
  Pencil,
  RefreshCw,
  Square,
  ArrowDown,
  History,
  LifeBuoy,
  Loader2,
  Plus,
  SendHorizontal,
} from "lucide-react";
import { usePathname } from "next/navigation";
import { useEffect, useRef, useState } from "react";

import { MarkdownContent } from "@/components/markdown/markdown-content";
import { Button } from "@/components/ui/button";
import { ScrollArea } from "@/components/ui/scroll-area";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { Textarea } from "@/components/ui/textarea";
import { useOptionalTenant } from "@/features/tenant";
import { isApiError } from "@/lib/api";
import { guidesService } from "@/features/guides/services/guides.service";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import {
  assistantService,
  type AssistantAction,
  type AssistantMessage,
} from "../services/assistant.service";

import {
  ActionCard,
  RouteLink,
  type ActionDecision,
  type EditedParams,
} from "./action-card";
import { AssistantMarkdown } from "./assistant-markdown";
import { DocumentAttach } from "./document-attach";

type Pending = { prompt?: string; guideKey?: string; nonce: number };

const keys = {
  status: (slug: string) => ["assistant", slug, "status"] as const,
  threads: (slug: string) => ["assistant", slug, "threads"] as const,
  messages: (slug: string, thread: string) =>
    ["assistant", slug, "messages", thread] as const,
};

function errorKey(err: unknown): string {
  if (isApiError(err)) {
    switch (err.code) {
      case "ASSISTANT_DISABLED":
        return "assistant.errors.disabled";
      case "ASSISTANT_DAILY_LIMIT":
        return "assistant.errors.daily_limit";
      case "ASSISTANT_AI_UNAVAILABLE":
        return "assistant.errors.ai_unavailable";
      case "ASSISTANT_SUPPORT_UNAVAILABLE":
        return "assistant.errors.support_unavailable";
      case "ASSISTANT_MESSAGE_REJECTED":
        return "assistant.errors.rejected";
      case "ASSISTANT_ACTION_NOT_PENDING":
        return "assistant.errors.not_pending";
      case "FORBIDDEN":
        return "assistant.errors.forbidden";
      case "RATE_LIMITED":
        return "assistant.errors.rate_limited";
    }
  }
  return "assistant.errors.generic";
}

/**
 * The ClickFix Asistan panel: threads, markdown answers, quick replies,
 * ProposedAction cards (confirm / reject) and hand-over to ClickFix support.
 */
export function AssistantDrawer({
  open,
  onOpenChange,
  pending,
  onPendingHandled,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  pending: Pending | null;
  onPendingHandled: () => void;
}) {
  const { t, locale } = useLocale();
  const slug = useOptionalTenant()?.slug ?? "";
  const pathname = usePathname() || "/";
  const qc = useQueryClient();
  const [thread, setThread] = useState<string | null>(null);
  const [showHistory, setShowHistory] = useState(false);
  const [draft, setDraft] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [escalating, setEscalating] = useState(false);
  const [note, setNote] = useState("");
  const [guideKey, setGuideKey] = useState<string | undefined>();
  const endRef = useRef<HTMLDivElement>(null);
  const composerRef = useRef<HTMLTextAreaElement>(null);
  const controller = useRef<AbortController | null>(null);
  const [live, setLive] = useState<AssistantMessage | null>(null);
  const [statusText, setStatusText] = useState("");
  const follow = useRef(true);
  const [jump, setJump] = useState(false);
  const lastSent = useRef("");
  const stop = () => controller.current?.abort();
  useEffect(() => {
    if (!open) controller.current?.abort();
  }, [open]);
  useEffect(() => () => controller.current?.abort(), []);

  const status = useQuery({
    queryKey: keys.status(slug),
    queryFn: () => assistantService.status(slug),
    enabled: open && Boolean(slug),
  });
  const threads = useQuery({
    queryKey: keys.threads(slug),
    queryFn: () => assistantService.threads(slug),
    enabled: open && showHistory && Boolean(slug),
  });
  const messages = useQuery({
    queryKey: keys.messages(slug, thread ?? ""),
    queryFn: () => assistantService.messages(slug, thread ?? ""),
    enabled: open && Boolean(slug && thread),
  });
  const pageGuides = useQuery({
    queryKey: ["guides", slug, "route", pathname, locale],
    queryFn: () => guidesService.forRoute(slug, pathname, locale),
    enabled: open && Boolean(slug) && !thread,
  });
  const pageTitle = pageGuides.data?.items.find(
    (g) => g.kind === "page",
  )?.title;
  const items = [...(messages.data?.items ?? []), ...(live ? [live] : [])];

  const context = { surface: "web" as const, route: pathname };

  const send = useMutation({
    mutationFn: async (input: {
      content: string;
      thread: string | null;
      guideKey?: string;
    }) => {
      const abort = new AbortController();
      controller.current = abort;
      lastSent.current = input.content;
      let id = input.thread;
      if (!id) {
        const created = await assistantService.createThread(
          slug,
          {
            ...context,
            guide_key: input.guideKey,
          },
          abort.signal,
        );
        id = created.uuid;
        setThread(id);
      }
      follow.current = true;
      setStatusText(t("assistant.thinking"));
      const replyId = "__stream";
      let savedUser = "";
      try {
        await assistantService.stream(
          slug,
          id,
          input.content,
          { ...context, guide_key: input.guideKey },
          locale,
          abort.signal,
          (name, data) => {
            if (name === "message.created") {
              const message = data as AssistantMessage;
              savedUser = message.uuid;
              qc.setQueryData<{ items: AssistantMessage[] }>(
                keys.messages(slug, id!),
                (old) => ({
                  items: [
                    ...(old?.items ?? []).filter(
                      (m) => m.uuid !== message.uuid,
                    ),
                    message,
                  ],
                }),
              );
              setLive({
                uuid: replyId,
                role: "assistant",
                content: "",
                created_at: new Date().toISOString(),
              });
            } else if (name === "status")
              setStatusText((data as { text: string }).text);
            else if (name === "delta")
              setLive((old) =>
                old
                  ? {
                      ...old,
                      content: old.content + (data as { text: string }).text,
                    }
                  : old,
              );
            else if (name === "suggestions")
              setLive((old) =>
                old ? { ...old, suggestions: data as string[] } : old,
              );
            else if (name === "action")
              setLive((old) =>
                old
                  ? {
                      ...old,
                      actions: [
                        ...(old.actions ?? []),
                        data as AssistantAction,
                      ],
                    }
                  : old,
              );
            else if (name === "done") {
              const reply = (data as { reply: AssistantMessage }).reply;
              qc.setQueryData<{ items: AssistantMessage[] }>(
                keys.messages(slug, id!),
                (old) => ({
                  items: [
                    ...(old?.items ?? []).filter((m) => m.uuid !== reply.uuid),
                    reply,
                  ],
                }),
              );
              setLive(null);
            }
          },
        );
      } catch (err) {
        if (!abort.signal.aborted) throw err;
      } finally {
        controller.current = null;
        if (savedUser) {
          for (let attempt = 0; attempt < 12; attempt++) {
            const data = await assistantService
              .messages(slug, id)
              .catch(() => null);
            const index =
              data?.items.findIndex((m) => m.uuid === savedUser) ?? -1;
            if (
              data &&
              index >= 0 &&
              data.items.slice(index + 1).some((m) => m.role === "assistant")
            ) {
              qc.setQueryData(keys.messages(slug, id), data);
              break;
            }
            await new Promise((resolve) => setTimeout(resolve, 150));
          }
        }
        setLive(null);
        await qc.invalidateQueries({ queryKey: keys.messages(slug, id) });
      }
      return id;
    },
    onMutate: () => {
      setError(null);
      setLive(null);
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: keys.status(slug) });
      void qc.invalidateQueries({ queryKey: keys.threads(slug) });
      requestAnimationFrame(() => composerRef.current?.focus());
    },
    onError: (err, input) => {
      controller.current = null;
      if (err instanceof DOMException && err.name === "AbortError") return;
      setError(errorKey(err));
      setDraft(input.content);
    },
  });

  // Secrets an action generated: shown once on its card, never stored.
  const [secrets, setSecrets] = useState<
    Record<string, Record<string, string>>
  >({});

  const decide = useMutation({
    mutationFn: async ({ action, confirm, params }: ActionDecision) => {
      if (!thread) return;
      if (confirm) {
        const out = await assistantService.confirm(
          slug,
          thread,
          action.id,
          locale,
          params,
        );
        const once = out.secret_once;
        if (once && Object.keys(once).length) {
          setSecrets((s) => ({ ...s, [action.id]: once }));
        }
      } else {
        await assistantService.reject(slug, thread, action.id);
      }
    },
    onMutate: () => setError(null),
    onError: (err) => setError(errorKey(err)),
    onSettled: () =>
      qc.invalidateQueries({ queryKey: keys.messages(slug, thread ?? "") }),
  });

  const dryRun = useMutation({
    mutationFn: async ({
      action,
      params,
    }: {
      action: AssistantAction;
      params?: EditedParams;
    }) => {
      if (!thread) return;
      await assistantService.dryRun(slug, thread, action.id, locale, params);
    },
    onMutate: () => setError(null),
    onError: (err) => setError(errorKey(err)),
    onSettled: () =>
      qc.invalidateQueries({ queryKey: keys.messages(slug, thread ?? "") }),
  });

  const escalate = useMutation({
    mutationFn: async () => {
      let id = thread;
      if (!id) {
        id = (await assistantService.createThread(slug, context)).uuid;
        setThread(id);
      }
      await assistantService.escalate(slug, id, note.trim(), locale);
      return id;
    },
    onMutate: () => setError(null),
    onSuccess: (id) => {
      setEscalating(false);
      setNote("");
      void qc.invalidateQueries({ queryKey: keys.messages(slug, id) });
    },
    onError: (err) => setError(errorKey(err)),
  });

  // A question from a tour step or the "?" popover opens a fresh thread.
  const handled = useRef<number>(0);
  useEffect(() => {
    if (!open || !pending || handled.current === pending.nonce || !slug) {
      return;
    }
    handled.current = pending.nonce;
    onPendingHandled();
    setGuideKey(pending.guideKey);
    setShowHistory(false);
    setThread(null);
    if (pending.prompt) {
      send.mutate({
        content: pending.prompt,
        thread: null,
        guideKey: pending.guideKey,
      });
    }
  }, [onPendingHandled, open, pending, send, slug]);

  useEffect(() => {
    if (follow.current) endRef.current?.scrollIntoView({ block: "end" });
  }, [items.length, live?.content, send.isPending]);

  // Documents need a thread: the first one opens it.
  const ensureThread = async () => {
    if (thread) return thread;
    const id = (await assistantService.createThread(slug, context)).uuid;
    setThread(id);
    return id;
  };

  const submit = (text: string) => {
    const content = text.trim();
    if (!content || send.isPending) return;
    setDraft("");
    send.mutate({ content, thread, guideKey });
  };

  const lastAssistant = [...items]
    .reverse()
    .find((m) => m.role === "assistant");
  const suggestions =
    items.length === 0
      ? [
          pageTitle
            ? t("assistant.starters.context", { title: pageTitle })
            : t("assistant.starters.page"),
          t("assistant.starters.setup"),
          t("assistant.starters.invite"),
          t("assistant.starters.problem"),
        ]
      : (lastAssistant?.suggestions ?? []);
  const disabled = !status.data?.enabled || status.isError;

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        side="right"
        className="flex w-full flex-col gap-0 p-0 sm:max-w-xl"
        onOpenAutoFocus={(e) => {
          e.preventDefault();
          composerRef.current?.focus();
        }}
        onEscapeKeyDown={(e) => {
          if (send.isPending) {
            e.preventDefault();
            stop();
          }
        }}
      >
        <SheetHeader className="border-b px-4 py-3">
          <div className="flex items-center gap-2 pr-8">
            <span className="bg-primary/10 text-primary flex size-8 items-center justify-center rounded-lg">
              <Bot className="size-4" />
            </span>
            <div className="min-w-0 flex-1">
              <SheetTitle className="text-base">
                {t("assistant.title")}
              </SheetTitle>
              <SheetDescription className="truncate text-xs">
                {t("assistant.subtitle")}
              </SheetDescription>
            </div>
            <Button
              variant="ghost"
              size="icon"
              className="size-8"
              aria-label={t("assistant.history")}
              aria-pressed={showHistory}
              onClick={() => setShowHistory((s) => !s)}
            >
              <History className="size-4" />
            </Button>
            <Button
              variant="ghost"
              size="icon"
              className="size-8"
              aria-label={t("assistant.new_chat")}
              onClick={() => {
                stop();
                setThread(null);
                setShowHistory(false);
                setError(null);
                setDraft("");
                requestAnimationFrame(() => composerRef.current?.focus());
              }}
            >
              <Plus className="size-4" />
            </Button>
          </div>
        </SheetHeader>

        {showHistory ? (
          <ScrollArea className="min-h-0 flex-1">
            {threads.isPending ? (
              <p role="status" className="text-muted-foreground p-6 text-sm">
                {t("common.loading")}
              </p>
            ) : null}
            {threads.isError ? (
              <QueryFailure onRetry={() => void threads.refetch()} />
            ) : null}
            <ul className="divide-y">
              {(threads.data?.items ?? []).map((th) => (
                <li key={th.uuid}>
                  <button
                    type="button"
                    className={cn(
                      "hover:bg-muted focus-visible:ring-ring w-full px-4 py-3 text-left text-sm focus-visible:ring-2 focus-visible:outline-none focus-visible:ring-inset",
                      th.uuid === thread && "bg-muted",
                    )}
                    aria-current={th.uuid === thread ? "true" : undefined}
                    onClick={() => {
                      stop();
                      setThread(th.uuid);
                      setShowHistory(false);
                    }}
                  >
                    <span className="block truncate font-medium">
                      {th.title || t("assistant.untitled")}
                    </span>
                    <span className="text-muted-foreground text-xs">
                      {new Date(th.updated_at).toLocaleString(locale)}
                    </span>
                  </button>
                </li>
              ))}
              {threads.data && threads.data.items.length === 0 ? (
                <li className="text-muted-foreground px-4 py-6 text-sm">
                  {t("assistant.no_history")}
                </li>
              ) : null}
            </ul>
          </ScrollArea>
        ) : (
          <ScrollArea
            className="min-h-0 flex-1"
            onScrollCapture={(e) => {
              const el = e.target as HTMLElement;
              follow.current =
                el.scrollHeight - el.scrollTop - el.clientHeight < 100;
              setJump(!follow.current);
            }}
          >
            <div className="space-y-3 p-4">
              {status.isPending || (thread && messages.isPending) ? (
                <p role="status" className="text-muted-foreground py-4 text-sm">
                  {t("common.loading")}
                </p>
              ) : null}
              {status.isError ? (
                <QueryFailure onRetry={() => void status.refetch()} />
              ) : null}
              {thread && messages.isError ? (
                <QueryFailure onRetry={() => void messages.refetch()} />
              ) : null}
              {items.length === 0 &&
              !send.isPending &&
              !messages.isFetching &&
              !messages.isError ? (
                <div className="bg-card rounded-2xl border p-5 text-sm shadow-xs">
                  <p className="font-medium">{t("assistant.welcome_title")}</p>
                  <p className="text-muted-foreground mt-1">
                    {t("assistant.welcome_body")}
                  </p>
                </div>
              ) : null}
              {items.map((m) => (
                <MessageBubble
                  key={m.uuid}
                  message={m}
                  busy={decide.isPending || dryRun.isPending}
                  secrets={secrets}
                  onEdit={
                    !send.isPending &&
                    m.uuid ===
                      [...items].reverse().find((x) => x.role === "user")?.uuid
                      ? () => {
                          setDraft(m.content);
                          composerRef.current?.focus();
                        }
                      : undefined
                  }
                  onDecide={(d) => decide.mutate(d)}
                  onDryRun={(action, params) =>
                    dryRun.mutate({ action, params })
                  }
                />
              ))}
              {send.isPending ? (
                <div
                  role="status"
                  className="text-muted-foreground bg-muted/50 flex items-center gap-2 rounded-xl p-3 text-sm"
                >
                  <Loader2 className="size-4 animate-spin motion-reduce:animate-none" />
                  {statusText || t("assistant.thinking")}
                </div>
              ) : null}
              <div ref={endRef} />
            </div>
          </ScrollArea>
        )}

        <div className="space-y-2 border-t p-3">
          {jump && !showHistory ? (
            <Button
              variant="secondary"
              size="sm"
              onClick={() => {
                follow.current = true;
                setJump(false);
                endRef.current?.scrollIntoView({ block: "end" });
              }}
            >
              <ArrowDown className="size-4" />
              {t("assistant.latest")}
            </Button>
          ) : null}
          {!send.isPending && lastAssistant ? (
            <Button
              variant="ghost"
              size="sm"
              onClick={() =>
                submit(
                  [...items].reverse().find((m) => m.role === "user")
                    ?.content ?? lastSent.current,
                )
              }
            >
              <RefreshCw className="size-4" />
              {t("assistant.regenerate")}
            </Button>
          ) : null}
          {disabled ? (
            <p className="text-muted-foreground text-xs">
              {t(`assistant.status.${status.data?.reason ?? "disabled"}`)}
            </p>
          ) : null}
          {error ? (
            <p role="alert" className="text-destructive text-xs">
              {t(error)}{" "}
              <button
                type="button"
                className="underline"
                onClick={() => submit(lastSent.current)}
              >
                {t("common.retry")}
              </button>
            </p>
          ) : null}
          {!showHistory && suggestions.length > 0 && !disabled ? (
            <div
              className={
                items.length === 0
                  ? "grid grid-cols-2 gap-2 pb-1"
                  : "flex gap-1.5 overflow-x-auto pb-1"
              }
            >
              {suggestions.map((s) => (
                <button
                  key={s}
                  type="button"
                  disabled={send.isPending}
                  onClick={() => submit(s)}
                  className={cn(
                    "hover:bg-muted focus-visible:ring-ring border px-3 py-2 text-xs focus-visible:ring-2 focus-visible:outline-none disabled:opacity-50",
                    items.length === 0
                      ? "rounded-xl text-left"
                      : "shrink-0 rounded-full",
                  )}
                >
                  {s}
                </button>
              ))}
            </div>
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
          <DocumentAttach
            slug={slug}
            thread={thread}
            ensureThread={ensureThread}
            disabled={Boolean(disabled) || send.isPending}
          />
          <form
            className="flex items-end gap-2"
            onSubmit={(e) => {
              e.preventDefault();
              submit(draft);
            }}
          >
            <Textarea
              ref={composerRef}
              rows={2}
              maxLength={4000}
              value={draft}
              disabled={Boolean(disabled)}
              onChange={(e) => setDraft(e.target.value)}
              onKeyDown={(e) => {
                if (
                  e.key === "Enter" &&
                  !e.shiftKey &&
                  !e.nativeEvent.isComposing
                ) {
                  e.preventDefault();
                  submit(draft);
                }
              }}
              placeholder={t("assistant.placeholder")}
              aria-label={t("assistant.placeholder")}
              className="max-h-40 min-h-10 resize-none"
            />
            {send.isPending ? (
              <Button
                type="button"
                size="icon"
                aria-label={t("assistant.stop")}
                onClick={stop}
              >
                <Square className="size-4" />
              </Button>
            ) : (
              <Button
                type="submit"
                size="icon"
                disabled={!draft.trim() || send.isPending || Boolean(disabled)}
                aria-label={t("assistant.send")}
              >
                <SendHorizontal className="size-4" />
              </Button>
            )}
          </form>
          <div className="text-muted-foreground flex items-center justify-between text-[11px]">
            {/* No support organization/inbox: escalation would answer 409
                ASSISTANT_SUPPORT_UNAVAILABLE, so the action is hidden. */}
            {status.data?.escalation_available === false ? (
              <span />
            ) : (
              <button
                type="button"
                className="hover:text-foreground inline-flex items-center gap-1"
                onClick={() => setEscalating((v) => !v)}
              >
                <LifeBuoy className="size-3.5" />
                {t("assistant.escalate.button")}
              </button>
            )}
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
      </SheetContent>
    </Sheet>
  );
}

function MessageBubble({
  message,
  busy,
  secrets,
  onDecide,
  onDryRun,
  onEdit,
}: {
  onEdit?: () => void;
  message: AssistantMessage;
  busy: boolean;
  secrets: Record<string, Record<string, string>>;
  onDecide: (d: ActionDecision) => void;
  onDryRun: (action: AssistantAction, params?: EditedParams) => void;
}) {
  const { t } = useLocale();
  const [copied, setCopied] = useState(false);
  if (message.role === "tool") {
    return (
      <div className="text-muted-foreground bg-muted/40 rounded-md px-3 py-2 text-xs">
        <MarkdownContent markdown={message.content} className="[&_p]:my-0" />
        {message.actions?.map((a) =>
          a.focus ? (
            <RouteLink
              key={a.id}
              route={a.focus.route}
              target={a.focus.target}
            />
          ) : a.route ? (
            <RouteLink key={a.id} route={a.route} />
          ) : null,
        )}
      </div>
    );
  }
  const mine = message.role === "user";
  return (
    <div className={cn("flex flex-col", mine ? "items-end" : "items-start")}>
      <div
        className={cn(
          "max-w-[92%] min-w-0 overflow-x-auto rounded-2xl px-4 py-3 text-sm break-words",
          mine ? "bg-primary text-primary-foreground" : "bg-muted",
        )}
      >
        {mine ? (
          <p className="whitespace-pre-wrap">{message.content}</p>
        ) : (
          <AssistantMarkdown text={message.content} />
        )}
      </div>
      <div className="flex gap-1">
        <Button
          variant="ghost"
          size="sm"
          aria-label={t("assistant.copy")}
          onClick={() => {
            void navigator.clipboard
              .writeText(message.content)
              .then(() => setCopied(true))
              .catch(() => undefined);
          }}
        >
          <Copy className="size-3" />
          {copied ? t("assistant.copied") : t("assistant.copy")}
        </Button>
        {onEdit ? (
          <Button variant="ghost" size="sm" onClick={onEdit}>
            <Pencil className="size-3" />
            {t("assistant.edit")}
          </Button>
        ) : null}
        {message.interrupted ? (
          <span className="text-muted-foreground self-center text-xs">
            {t("assistant.interrupted")}
          </span>
        ) : null}
      </div>
      {message.actions?.map((a) => (
        <ActionCard
          key={a.id}
          action={a}
          busy={busy}
          secrets={secrets[a.id]}
          onDecide={onDecide}
          onDryRun={onDryRun}
        />
      ))}
    </div>
  );
}

function QueryFailure({ onRetry }: { onRetry: () => void }) {
  const { t } = useLocale();
  return (
    <div
      role="alert"
      className="bg-destructive/5 text-destructive border-destructive/20 m-3 space-y-2 rounded-xl border p-3 text-sm"
    >
      <p>{t("assistant.errors.generic")}</p>
      <Button variant="outline" size="sm" onClick={onRetry}>
        {t("common.retry")}
      </Button>
    </div>
  );
}
