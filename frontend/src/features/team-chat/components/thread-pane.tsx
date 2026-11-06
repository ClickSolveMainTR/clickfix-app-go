"use client";

import { ArrowLeft, Loader2, SendHorizontal, Sparkles } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";

import { Loading } from "@/components/common/loading";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { initials, shortUntil } from "@/features/inbox/lib/format";
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
import {
  insertMention,
  mentionQueryAt,
  serializeMentions,
  type Mention,
} from "../lib/mentions";

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
  const { t, locale } = useLocale();
  const { user } = useAuth();
  const presence = useTeamPresence();
  const send = useSendThreadMessage(uuid);
  const ask = useAskAi(uuid);
  const area = useRef<HTMLTextAreaElement>(null);
  // Caret to restore once a mention pick has rewritten the text.
  const pendingCaret = useRef<number | null>(null);
  const [text, setText] = useState("");
  const [mentions, setMentions] = useState<Mention[]>([]);
  const [mentionQuery, setMentionQuery] = useState<string | null>(null);
  const [mentionIndex, setMentionIndex] = useState(0);
  const busy = send.isPending || ask.isPending;
  const ok = Boolean(text.trim()) && !busy;

  // Same source as mobile's picker in spirit: teammates of the organization.
  // The backend only accepts members of the thread, so mentioning someone
  // outside it fails with the API's message.
  const candidates = useMemo(() => {
    if (mentionQuery == null) return [];
    const q = mentionQuery.toLocaleLowerCase(locale);
    return (presence.data ?? [])
      .filter(
        (m) =>
          m.user_uuid !== user?.uuid &&
          m.name.toLocaleLowerCase(locale).includes(q),
      )
      .slice(0, 6);
  }, [presence.data, mentionQuery, locale, user?.uuid]);

  useEffect(() => {
    const el = area.current;
    const caret = pendingCaret.current;
    if (!el || caret == null) return;
    pendingCaret.current = null;
    if (document.activeElement !== el) el.focus();
    el.setSelectionRange(caret, caret);
  }, [text]);

  const fail = (e: unknown) =>
    appToast.error(isApiError(e) ? e.message : t("team_chat.failed"));
  const clear = () => {
    setText("");
    setMentions([]);
    setMentionQuery(null);
  };
  const submit = () => {
    if (!ok) return;
    send.mutate(
      { body: text.trim(), mentions: serializeMentions(text, mentions) },
      { onSuccess: clear, onError: fail },
    );
  };
  // Same as mobile: what is typed goes to the AI as the question; the answer
  // lands in the thread as an AI message every member sees.
  const askAi = () => {
    if (ok) ask.mutate(text.trim(), { onSuccess: clear, onError: fail });
  };
  const pick = (m: { user_uuid: string; name: string }) => {
    const el = area.current;
    if (!el) return;
    const next = insertMention(text, el.selectionStart, m.name);
    setText(next.text);
    setMentions((cur) =>
      cur.some((x) => x.uuid === m.user_uuid)
        ? cur
        : [...cur, { uuid: m.user_uuid, name: m.name }],
    );
    setMentionQuery(null);
    pendingCaret.current = next.caret;
  };
  const onKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (mentionQuery != null && candidates.length > 0) {
      if (e.key === "ArrowDown" || e.key === "ArrowUp") {
        e.preventDefault();
        const step = e.key === "ArrowDown" ? 1 : -1;
        setMentionIndex(
          (i) => (i + step + candidates.length) % candidates.length,
        );
        return;
      }
      if (e.key === "Enter" || e.key === "Tab") {
        e.preventDefault();
        const member = candidates[mentionIndex];
        if (member) pick(member);
        return;
      }
      if (e.key === "Escape") {
        e.preventDefault();
        setMentionQuery(null);
        return;
      }
    }
    if (e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) {
      e.preventDefault();
      submit();
    }
  };

  return (
    <div className="relative flex items-end gap-2 border-t p-3">
      {mentionQuery != null && candidates.length > 0 && (
        <div
          role="listbox"
          aria-label={t("team_chat.mention")}
          className="bg-popover absolute bottom-full left-14 z-20 mb-1 w-64 overflow-hidden rounded-lg border p-1 shadow-lg"
        >
          {candidates.map((m, i) => (
            <button
              type="button"
              key={m.user_uuid}
              role="option"
              aria-selected={i === mentionIndex}
              onMouseDown={(e) => {
                e.preventDefault();
                pick(m);
              }}
              className={cn(
                "flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm",
                i === mentionIndex ? "bg-accent" : "hover:bg-muted",
              )}
            >
              <span className="bg-muted flex size-6 items-center justify-center rounded-full text-[10px] font-semibold">
                {initials(m.name)}
              </span>
              <span className="truncate">{m.name}</span>
            </button>
          ))}
        </div>
      )}
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
        ref={area}
        value={text}
        onChange={(e) => {
          setText(e.target.value);
          setMentionQuery(
            mentionQueryAt(e.target.value, e.target.selectionStart),
          );
          setMentionIndex(0);
        }}
        onKeyDown={onKeyDown}
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
