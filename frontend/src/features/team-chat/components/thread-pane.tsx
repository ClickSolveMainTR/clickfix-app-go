"use client";

import { ArrowLeft, Loader2, SendHorizontal, Sparkles } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";

import { Loading } from "@/components/common/loading";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { shortUntil } from "@/features/inbox/lib/format";
import { useTeamPresence } from "@/features/presence/hooks/use-presence";
import { isApiError } from "@/lib/api";
import { cn } from "@/lib/utils";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

import {
  useAskAi,
  useMarkThreadRead,
  useSendThreadMessage,
  useThreadMessages,
} from "../hooks/use-team-chat";

/**
 * One team chat thread: messages (live over the user's realtime channel),
 * reply, and "Ask AI" with the organization's knowledge. Opening the thread
 * and every new message while it is open mark it read.
 */
export function ThreadPane({
  uuid,
  title,
  subtitle,
  closed,
  onBack,
}: {
  uuid: string;
  title: string;
  subtitle: string;
  closed: boolean;
  onBack: () => void;
}) {
  const { t, locale } = useLocale();
  const { user } = useAuth();
  const messages = useThreadMessages(uuid);
  const presence = useTeamPresence();
  const names = useMemo(
    () => new Map((presence.data ?? []).map((m) => [m.user_uuid, m.name])),
    [presence.data],
  );
  const items = messages.items;
  const newest = items[items.length - 1]?.uuid;
  useMarkThreadRead(uuid, newest);

  const scroller = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const el = scroller.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [newest]);

  return (
    <>
      <header className="flex items-center gap-2 border-b px-3 py-2.5">
        <Button
          variant="ghost"
          size="icon"
          className="md:hidden"
          onClick={onBack}
          aria-label={t("team_chat.back")}
        >
          <ArrowLeft className="size-4" />
        </Button>
        <div className="min-w-0">
          <h2 className="truncate text-sm font-semibold">{title}</h2>
          <p className="text-muted-foreground truncate text-xs">
            {[subtitle, closed ? t("team_chat.closed") : null]
              .filter(Boolean)
              .join(" · ")}
          </p>
        </div>
      </header>

      <div
        ref={scroller}
        className="min-h-0 flex-1 overflow-y-auto p-3"
        aria-live="polite"
      >
        {messages.isLoading ? (
          <Loading />
        ) : messages.isError ? (
          <div className="flex h-full flex-col items-center justify-center gap-2 text-center">
            <p className="text-muted-foreground text-sm">
              {t("team_chat.load_failed")}
            </p>
            <Button
              size="sm"
              variant="outline"
              onClick={() => void messages.refetch()}
            >
              {t("team_chat.retry")}
            </Button>
          </div>
        ) : items.length === 0 ? (
          <p className="text-muted-foreground flex h-full items-center justify-center text-sm">
            {t("team_chat.no_messages")}
          </p>
        ) : (
          <ul className="flex flex-col gap-2">
            {items.map((m) => {
              const mine = m.sender_uuid === user?.uuid && !m.is_ai;
              return (
                <li
                  key={m.uuid}
                  className={cn(
                    "flex max-w-[85%] flex-col gap-1 sm:max-w-[560px]",
                    mine ? "items-end self-end" : "items-start self-start",
                  )}
                >
                  <div
                    className={cn(
                      "rounded-2xl px-3.5 py-2 text-sm break-words whitespace-pre-wrap",
                      mine
                        ? "bg-primary text-primary-foreground"
                        : m.is_ai
                          ? "border-signal/50 bg-signal-soft border"
                          : "bg-card border",
                    )}
                  >
                    {m.body}
                  </div>
                  <span className="text-muted-foreground text-[11px]">
                    {[
                      mine
                        ? null
                        : m.is_ai
                          ? t("team_chat.ai")
                          : ((m.sender_uuid && names.get(m.sender_uuid)) ??
                            "—"),
                      shortUntil(m.created_at, locale),
                    ]
                      .filter(Boolean)
                      .join(" · ")}
                  </span>
                </li>
              );
            })}
          </ul>
        )}
      </div>

      {closed ? (
        <p className="text-muted-foreground border-t px-3 py-3 text-center text-xs">
          {t("team_chat.closed_hint")}
        </p>
      ) : (
        <ThreadComposer uuid={uuid} />
      )}
    </>
  );
}

function ThreadComposer({ uuid }: { uuid: string }) {
  const { t } = useLocale();
  const send = useSendThreadMessage(uuid);
  const ask = useAskAi(uuid);
  const [text, setText] = useState("");
  const busy = send.isPending || ask.isPending;
  const ok = Boolean(text.trim()) && !busy;
  const fail = (e: unknown) =>
    appToast.error(isApiError(e) ? e.message : t("team_chat.failed"));
  const clear = () => setText("");
  const submit = () => {
    if (ok) send.mutate(text.trim(), { onSuccess: clear, onError: fail });
  };
  // Same as mobile: what is typed goes to the AI as the question; the answer
  // lands in the thread as an AI message every member sees.
  const askAi = () => {
    if (ok) ask.mutate(text.trim(), { onSuccess: clear, onError: fail });
  };
  return (
    <div className="flex items-end gap-2 border-t p-3">
      <Button
        type="button"
        variant="secondary"
        size="icon"
        className="shrink-0 rounded-full"
        disabled={!ok}
        onClick={askAi}
        aria-label={t("team_chat.ask_ai")}
        title={t("team_chat.ask_ai")}
      >
        {ask.isPending ? (
          <Loader2 className="size-4 animate-spin" />
        ) : (
          <Sparkles className="size-4" />
        )}
      </Button>
      <Textarea
        value={text}
        onChange={(e) => setText(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) {
            e.preventDefault();
            submit();
          }
        }}
        rows={1}
        maxLength={4000}
        placeholder={t("team_chat.placeholder")}
        aria-label={t("team_chat.placeholder")}
        className="max-h-40 min-h-9 flex-1 resize-none"
      />
      <Button
        type="button"
        size="icon"
        className="shrink-0 rounded-full"
        disabled={!ok}
        onClick={submit}
        aria-label={t("team_chat.send")}
        title={t("team_chat.send")}
      >
        {send.isPending ? (
          <Loader2 className="size-4 animate-spin" />
        ) : (
          <SendHorizontal className="size-4" />
        )}
      </Button>
    </div>
  );
}
