"use client";

import {
  forwardRef,
  useCallback,
  useEffect,
  useImperativeHandle,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { useQuery } from "@tanstack/react-query";
import {
  Loader2,
  Lock,
  Paperclip,
  SendHorizontal,
  Sparkles,
  X,
} from "lucide-react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { cn, fileSize } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { useCannedResponses } from "@/features/macros/hooks/use-macros";
import {
  macrosService,
  type Canned,
} from "@/features/macros/services/macros.service";

import {
  useAIAvailability,
  useCopilot,
  type CopilotEntry,
  type CopilotRequest,
} from "../hooks/use-copilot";
import { useInboxAgents, useSendMessage } from "../hooks/use-inbox";
import { aiCodeKey } from "../lib/copilot";
import { useDrafts } from "../lib/drafts";
import { initials } from "../lib/format";
import { copilotService } from "../services/copilot.service";
import { inboxService } from "../services/inbox.service";
import { CopilotMenu } from "./copilot-menu";
import { CopilotPanel, type CopilotApply } from "./copilot-panel";
import type { ConversationDetail, InboxAgent } from "../types";

export type ComposerHandle = {
  focus: (mode?: "reply" | "note") => void;
  /** Copilot shortcuts of the inbox (G suggest, U summarize, Q ask). */
  copilot: (op: "suggest_reply" | "summarize" | "ask") => void;
};

type Upload = { id: string; file: File; uuid?: string; failed?: boolean };

type Props = {
  conversation: ConversationDetail;
  canReply: boolean;
  canNote: boolean;
  /** Reports whether the agent is composing (presence "typing"). */
  onTypingChange?: (typing: boolean) => void;
  /** Agent copilot (CF-66): needs the reply permission. */
  canCopilot?: boolean;
};

const MAX_HEIGHT = 240;

/**
 * Reply / private note editor. Drafts survive reloads (per conversation),
 * ⌘/Ctrl+Enter sends (the parent keys it by conversation, so state
 * starts from that conversation's draft), `@` in a note mentions a teammate, files upload
 * while typing and attach on send. Sending is optimistic.
 */
export const Composer = forwardRef<ComposerHandle, Props>(function Composer(
  { conversation, canReply, canNote, onTypingChange, canCopilot = canReply },
  ref,
) {
  const { t } = useLocale();
  const uuid = conversation.uuid;
  const draft = useDrafts((s) => s.drafts[uuid]);
  const saveDraft = useDrafts((s) => s.set);
  const clearDraft = useDrafts((s) => s.clear);
  const [text, setText] = useState(draft?.text ?? "");
  const [mode, setMode] = useState<"reply" | "note">(
    draft?.mode ?? (canReply ? "reply" : "note"),
  );
  const [uploads, setUploads] = useState<Upload[]>([]);
  const [mentions, setMentions] = useState<InboxAgent[]>([]);
  const [mentionQuery, setMentionQuery] = useState<string | null>(null);
  // "/query" typed at the start of a line opens the canned response picker.
  const [cannedQuery, setCannedQuery] = useState<string | null>(null);
  const [cannedIndex, setCannedIndex] = useState(0);
  const canned = useCannedResponses();
  const [mentionIndex, setMentionIndex] = useState(0);
  const area = useRef<HTMLTextAreaElement>(null);
  // Caret to restore once a programmatic text change (mention pick) is
  // committed. Restored in a layout effect, before the browser handles the
  // next keystroke; a requestAnimationFrame could run after it and move the
  // caret behind characters typed in between.
  const pendingCaret = useRef<number | null>(null);
  const fileInput = useRef<HTMLInputElement>(null);
  const send = useSendMessage(uuid);
  const agents = useInboxAgents();
  const copilot = useCopilot(uuid);
  // AI off / not configured / not in the plan: copilot actions are disabled
  // with the reason instead of failing on click.
  const aiAvailability = useAIAvailability(canCopilot);
  const aiBlocked =
    aiAvailability.data && !aiAvailability.data.available
      ? (aiAvailability.data.reason ?? "AI_DISABLED")
      : undefined;
  const copilotOn = canCopilot && !aiBlocked;
  const [menuOpen, setMenuOpen] = useState(false);
  const [asking, setAsking] = useState(false);
  const agentDraft = useAgentDraft(conversation, canCopilot && canReply);

  // Persist drafts shortly after typing stops.
  useEffect(() => {
    const id = setTimeout(() => saveDraft(uuid, { text, mode }), 300);
    return () => clearTimeout(id);
  }, [text, mode, uuid, saveDraft]);

  useLayoutEffect(() => {
    const el = area.current;
    const caret = pendingCaret.current;
    if (!el || caret == null) return;
    pendingCaret.current = null;
    if (document.activeElement !== el) el.focus();
    el.setSelectionRange(caret, caret);
  }, [text]);

  // Auto-grow.
  useEffect(() => {
    const el = area.current;
    if (!el) return;
    el.style.height = "auto";
    el.style.height = `${Math.min(el.scrollHeight, MAX_HEIGHT)}px`;
  }, [text]);

  const { run: copilotRun } = copilot;
  const runCopilot = useCallback(
    (req: CopilotRequest) => {
      setAsking(false);
      copilotRun(req);
    },
    [copilotRun],
  );

  const retryCopilot = (entry: CopilotEntry) =>
    runCopilot({
      op: entry.op,
      text: entry.input || undefined,
      ...(entry.op === "tone" ? { tone: entry.option } : {}),
      ...(entry.op === "translate" ? { locale: entry.option } : {}),
    });

  /** Puts a copilot result into the editor; replacing can be undone. */
  const applyCopilot = (value: string, how: CopilotApply) => {
    const previous = text;
    const previousMode = mode;
    const nextMode = how === "note" ? "note" : "reply";
    const next =
      how === "replace" || !previous.trim()
        ? value
        : `${previous.trimEnd()}\n\n${value}`;
    setMode(nextMode);
    setText(next);
    copilot.close();
    pendingCaret.current = next.length;
    if (previous.trim() && how === "replace") {
      toast(t("inbox.copilot.replaced"), {
        action: {
          label: t("inbox.copilot.undo"),
          onClick: () => {
            setMode(previousMode);
            setText(previous);
          },
        },
      });
    }
  };

  useImperativeHandle(ref, () => ({
    focus: (m) => {
      if (m) setMode(m);
      requestAnimationFrame(() => area.current?.focus());
    },
    copilot: (op) => {
      if (!copilotOn) return;
      if (op === "ask") {
        copilot.setOpen(true);
        setAsking(true);
        return;
      }
      runCopilot({ op });
    },
  }));

  const windowClosed = mode === "reply" && !conversation.window_open;
  const noChannel = mode === "reply" && !conversation.inbox;
  const uploading = uploads.some((u) => !u.uuid && !u.failed);
  const ready = uploads.filter((u) => u.uuid);
  const allowed = mode === "note" ? canNote : canReply;
  const canSend =
    allowed &&
    !windowClosed &&
    !noChannel &&
    !uploading &&
    (text.trim().length > 0 || ready.length > 0);

  const mentionCandidates = useMemo(() => {
    if (mentionQuery == null) return [];
    const q = mentionQuery.toLocaleLowerCase("tr");
    return (agents.data ?? [])
      .filter(
        (a) =>
          a.name.toLocaleLowerCase("tr").includes(q) ||
          a.email.toLowerCase().includes(q),
      )
      .slice(0, 6);
  }, [agents.data, mentionQuery]);

  // CF-49: when the contact wrote after the agent started composing, the
  // first send asks for a confirmation instead of sending.
  const [composeStart, setComposeStart] = useState<number | null>(null);
  const [confirmNew, setConfirmNew] = useState(false);
  const newSinceCompose =
    composeStart != null &&
    conversation.last_inbound_at != null &&
    new Date(conversation.last_inbound_at).getTime() > composeStart;

  const submit = useCallback(() => {
    if (!canSend) return;
    if (newSinceCompose && !confirmNew && mode === "reply") {
      setConfirmNew(true);
      return;
    }
    setConfirmNew(false);
    setComposeStart(null);
    onTypingChange?.(false);
    const content = text.trim();
    const used = mentions
      .filter((a) => content.includes(`@${a.name}`))
      .map((a) => a.uuid);
    send.mutate(
      {
        client_id: crypto.randomUUID(),
        content,
        private: mode === "note",
        ...(mode === "note" && used.length > 0 ? { mentions: used } : {}),
        ...(ready.length > 0
          ? { attachments: ready.map((u) => u.uuid as string) }
          : {}),
      },
      {
        onError: () => {
          setText((cur) => (cur.trim() ? cur : content));
        },
      },
    );
    setText("");
    setUploads([]);
    setMentions([]);
    clearDraft(uuid);
  }, [
    canSend,
    text,
    mentions,
    send,
    mode,
    ready,
    clearDraft,
    uuid,
    newSinceCompose,
    confirmNew,
    onTypingChange,
  ]);

  const pickMention = (agent: InboxAgent) => {
    const el = area.current;
    if (!el) return;
    const caret = el.selectionStart;
    const before = text
      .slice(0, caret)
      .replace(/@([^\s@]*)$/, `@${agent.name} `);
    const next = before + text.slice(caret);
    setText(next);
    setMentions((m) =>
      m.some((x) => x.uuid === agent.uuid) ? m : [...m, agent],
    );
    setMentionQuery(null);
    pendingCaret.current = before.length;
  };

  const cannedCandidates = useMemo(() => {
    if (cannedQuery == null) return [];
    const q = cannedQuery.toLocaleLowerCase("tr");
    return (canned.data ?? [])
      .filter(
        (c) =>
          c.shortcut.startsWith(q) ||
          c.title.toLocaleLowerCase("tr").includes(q),
      )
      .slice(0, 8);
  }, [canned.data, cannedQuery]);

  const pickCanned = async (item: Canned) => {
    const el = area.current;
    if (!el) return;
    const caret = el.selectionStart;
    setCannedQuery(null);
    let content = item.content;
    try {
      content = await macrosService.render(item.uuid, uuid);
    } catch {
      // Fall back to the raw text; the agent still sees the variables.
    }
    const before = text
      .slice(0, caret)
      .replace(/(^|\s)\/[^\s/]*$/, `$1${content}`);
    const next = before + text.slice(caret);
    setText(next);
    requestAnimationFrame(() => {
      el.focus();
      el.setSelectionRange(before.length, before.length);
    });
  };

  const onChange = (value: string, caret: number) => {
    setText(value);
    const slash = /(?:^|\n)\/([a-z0-9_-]{0,40})$/i.exec(value.slice(0, caret));
    setCannedQuery(slash ? (slash[1] ?? "").toLowerCase() : null);
    setCannedIndex(0);
    const composing = value.trim().length > 0;
    if (composing && composeStart == null) setComposeStart(Date.now());
    if (!composing) {
      setComposeStart(null);
      setConfirmNew(false);
    }
    onTypingChange?.(composing);
    if (mode !== "note") return;
    const match = /(?:^|\s)@([^\s@]{0,30})$/.exec(value.slice(0, caret));
    setMentionQuery(match ? (match[1] ?? "") : null);
    setMentionIndex(0);
  };

  const onKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (cannedQuery != null && cannedCandidates.length > 0) {
      if (e.key === "ArrowDown" || e.key === "ArrowUp") {
        e.preventDefault();
        const step = e.key === "ArrowDown" ? 1 : -1;
        setCannedIndex(
          (i) => (i + step + cannedCandidates.length) % cannedCandidates.length,
        );
        return;
      }
      if (e.key === "Enter" || e.key === "Tab") {
        e.preventDefault();
        const item = cannedCandidates[cannedIndex];
        if (item) void pickCanned(item);
        return;
      }
      if (e.key === "Escape") {
        e.preventDefault();
        setCannedQuery(null);
        return;
      }
    }
    if (mentionQuery != null && mentionCandidates.length > 0) {
      if (e.key === "ArrowDown" || e.key === "ArrowUp") {
        e.preventDefault();
        const step = e.key === "ArrowDown" ? 1 : -1;
        setMentionIndex(
          (i) =>
            (i + step + mentionCandidates.length) % mentionCandidates.length,
        );
        return;
      }
      if (e.key === "Enter" || e.key === "Tab") {
        e.preventDefault();
        const agent = mentionCandidates[mentionIndex];
        if (agent) pickMention(agent);
        return;
      }
      if (e.key === "Escape") {
        e.preventDefault();
        setMentionQuery(null);
        return;
      }
    }
    if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
      e.preventDefault();
      submit();
      return;
    }
    if (e.key === "." && (e.metaKey || e.ctrlKey) && copilotOn) {
      e.preventDefault();
      setMenuOpen(true);
      return;
    }
    if (e.key === "Escape") {
      e.preventDefault();
      area.current?.blur();
    }
  };

  const addFiles = (files: FileList | null) => {
    if (!files) return;
    for (const file of Array.from(files)) {
      const id = crypto.randomUUID();
      setUploads((u) => [...u, { id, file }]);
      inboxService
        .upload(file)
        .then((res) =>
          setUploads((u) =>
            u.map((x) => (x.id === id ? { ...x, uuid: res.uuid } : x)),
          ),
        )
        .catch(() => {
          setUploads((u) =>
            u.map((x) => (x.id === id ? { ...x, failed: true } : x)),
          );
          toast.error(t("inbox.composer.upload_failed", { name: file.name }));
        });
    }
  };

  return (
    <div className="bg-background border-t p-3">
      {canCopilot && (
        <CopilotPanel
          copilot={copilot}
          draftEmpty={!text.trim()}
          canNote={canNote}
          canReply={canReply}
          onApply={applyCopilot}
          onRetry={retryCopilot}
          onAsk={(question) => runCopilot({ op: "ask", text: question })}
          asking={asking}
          onAskingChange={(a) => {
            setAsking(a);
            if (!a && !copilot.current) copilot.setOpen(false);
          }}
        />
      )}
      {canCopilot && agentDraft && !copilot.open && !asking && (
        <button
          type="button"
          onClick={() =>
            copilot.showAgentDraft(agentDraft.uuid, agentDraft.text)
          }
          className="bg-signal-soft/60 hover:bg-signal-soft text-foreground mb-2 flex w-full items-center gap-2 rounded-lg border border-dashed px-3 py-1.5 text-left text-xs transition-colors"
        >
          <Sparkles className="text-signal-text size-3.5 shrink-0" />
          <span className="min-w-0 flex-1 truncate">
            {agentDraft.confidence != null
              ? t("inbox.copilot.agent_draft_ready_confidence", {
                  n: Math.round(agentDraft.confidence * 100),
                })
              : t("inbox.copilot.agent_draft_ready")}
          </span>
          <span className="text-signal-text font-medium">
            {t("inbox.copilot.agent_draft_show")}
          </span>
        </button>
      )}
      <div
        className={cn(
          "relative rounded-xl border transition-colors focus-within:ring-2",
          mode === "note"
            ? "border-amber-300 bg-amber-50/60 focus-within:ring-amber-300/50 dark:border-amber-800 dark:bg-amber-950/30"
            : "bg-card focus-within:ring-ring/40",
        )}
        onDragOver={(e) => e.preventDefault()}
        onDrop={(e) => {
          e.preventDefault();
          addFiles(e.dataTransfer.files);
        }}
      >
        <div className="flex items-center gap-1 px-2 pt-2">
          <ModeTab
            active={mode === "reply"}
            onClick={() => setMode("reply")}
            shortcut="R"
          >
            {t("inbox.composer.reply")}
          </ModeTab>
          <ModeTab
            active={mode === "note"}
            onClick={() => setMode("note")}
            shortcut="N"
          >
            <Lock className="size-3" />
            {t("inbox.composer.note")}
          </ModeTab>
          {mode === "reply" &&
            conversation.window_ends_at &&
            conversation.window_open && (
              <span className="text-muted-foreground ml-auto text-[11px]">
                {t("inbox.composer.window_until", {
                  time: new Date(
                    conversation.window_ends_at,
                  ).toLocaleTimeString([], {
                    hour: "2-digit",
                    minute: "2-digit",
                  }),
                })}
              </span>
            )}
        </div>
        {confirmNew && (
          <div
            role="alert"
            className="mx-3 mt-2 flex items-center gap-2 rounded-md bg-sky-100 px-2.5 py-1.5 text-xs text-sky-950 dark:bg-sky-900/40 dark:text-sky-100"
          >
            <span className="flex-1">
              {t("inbox.composer.new_message_warning")}
            </span>
            <button
              type="button"
              className="font-semibold underline-offset-2 hover:underline"
              onClick={submit}
            >
              {t("inbox.composer.send_anyway")}
            </button>
            <button
              type="button"
              className="opacity-70 hover:opacity-100"
              onClick={() => setConfirmNew(false)}
            >
              {t("inbox.composer.review")}
            </button>
          </div>
        )}
        {(windowClosed || noChannel) && (
          <p className="mx-3 mt-2 rounded-md bg-amber-100 px-2.5 py-1.5 text-xs text-amber-900 dark:bg-amber-900/40 dark:text-amber-100">
            {noChannel
              ? t("inbox.composer.no_channel")
              : t("inbox.composer.window_closed")}
          </p>
        )}
        <textarea
          ref={area}
          value={text}
          rows={2}
          disabled={!allowed}
          aria-label={
            mode === "note"
              ? t("inbox.composer.note")
              : t("inbox.composer.reply")
          }
          placeholder={
            mode === "note"
              ? t("inbox.composer.note_placeholder")
              : t("inbox.composer.reply_placeholder")
          }
          onChange={(e) => onChange(e.target.value, e.target.selectionStart)}
          onKeyDown={onKeyDown}
          onPaste={(e) => {
            if (e.clipboardData.files.length > 0) {
              e.preventDefault();
              addFiles(e.clipboardData.files);
            }
          }}
          className="placeholder:text-muted-foreground block w-full resize-none bg-transparent px-3 py-2 text-sm leading-relaxed outline-none"
        />
        {cannedQuery != null && cannedCandidates.length > 0 && (
          <div
            role="listbox"
            aria-label={t("inbox.composer.canned")}
            className="bg-popover absolute bottom-full left-3 z-20 mb-1 w-96 max-w-[calc(100%-1.5rem)] overflow-hidden rounded-lg border p-1 shadow-lg"
          >
            <p className="text-muted-foreground px-2 py-1 text-[11px]">
              {t("inbox.composer.canned_hint")}
            </p>
            {cannedCandidates.map((c, i) => (
              <button
                type="button"
                key={c.uuid}
                role="option"
                aria-selected={i === cannedIndex}
                onMouseDown={(e) => {
                  e.preventDefault();
                  void pickCanned(c);
                }}
                className={cn(
                  "flex w-full flex-col rounded-md px-2 py-1.5 text-left",
                  i === cannedIndex ? "bg-accent" : "hover:bg-muted",
                )}
              >
                <span className="flex items-center gap-2 text-sm">
                  <span className="text-muted-foreground font-mono text-xs">
                    /{c.shortcut}
                  </span>
                  <span className="truncate font-medium">{c.title}</span>
                </span>
                <span className="text-muted-foreground truncate text-xs">
                  {c.content}
                </span>
              </button>
            ))}
          </div>
        )}
        {mentionQuery != null && mentionCandidates.length > 0 && (
          <div
            role="listbox"
            className="bg-popover absolute bottom-full left-3 z-20 mb-1 w-64 overflow-hidden rounded-lg border p-1 shadow-lg"
          >
            {mentionCandidates.map((a, i) => (
              <button
                type="button"
                key={a.uuid}
                role="option"
                aria-selected={i === mentionIndex}
                onMouseDown={(e) => {
                  e.preventDefault();
                  pickMention(a);
                }}
                className={cn(
                  "flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm",
                  i === mentionIndex ? "bg-accent" : "hover:bg-muted",
                )}
              >
                <span className="bg-muted flex size-6 items-center justify-center rounded-full text-[10px] font-semibold">
                  {initials(a.name)}
                </span>
                <span className="truncate">{a.name}</span>
              </button>
            ))}
          </div>
        )}
        {uploads.length > 0 && (
          <div className="flex flex-wrap gap-1.5 px-3 pb-2">
            {uploads.map((u) => (
              <span
                key={u.id}
                className={cn(
                  "bg-background flex items-center gap-1.5 rounded-md border px-2 py-1 text-xs",
                  u.failed && "border-red-300 text-red-600",
                )}
              >
                {!u.uuid && !u.failed ? (
                  <Loader2 className="size-3 animate-spin" />
                ) : (
                  <Paperclip className="size-3" />
                )}
                <span className="max-w-40 truncate">{u.file.name}</span>
                <span className="text-muted-foreground">
                  {fileSize(u.file.size)}
                </span>
                <button
                  type="button"
                  aria-label={t("inbox.composer.remove_file")}
                  onClick={() =>
                    setUploads((x) => x.filter((y) => y.id !== u.id))
                  }
                >
                  <X className="size-3" />
                </button>
              </span>
            ))}
          </div>
        )}
        <div className="flex items-center gap-1 px-2 pb-2">
          <input
            ref={fileInput}
            type="file"
            multiple
            hidden
            onChange={(e) => {
              addFiles(e.target.files);
              e.target.value = "";
            }}
          />
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="size-8"
            disabled={!allowed}
            aria-label={t("inbox.composer.attach")}
            onClick={() => fileInput.current?.click()}
          >
            <Paperclip className="size-4" />
          </Button>
          {canCopilot && (
            <CopilotMenu
              open={menuOpen}
              onOpenChange={setMenuOpen}
              draft={text}
              disabled={Boolean(aiBlocked)}
              disabledReason={
                aiBlocked
                  ? t(aiCodeKey(aiBlocked) ?? "inbox.copilot.error.disabled")
                  : undefined
              }
              pending={copilot.pending}
              onRun={runCopilot}
              onAsk={() => {
                copilot.setOpen(true);
                setAsking(true);
              }}
            />
          )}
          <span className="text-muted-foreground ml-auto hidden text-[11px] sm:inline">
            {t("inbox.composer.send_hint")}
          </span>
          <Button
            type="button"
            size="sm"
            disabled={!canSend}
            onClick={submit}
            className={cn(
              mode === "note" &&
                "bg-amber-500 text-amber-950 hover:bg-amber-500/90",
            )}
          >
            {mode === "note"
              ? t("inbox.composer.add_note")
              : t("inbox.composer.send")}
            <SendHorizontal className="size-3.5" />
          </Button>
        </div>
      </div>
    </div>
  );
});

function ModeTab({
  active,
  onClick,
  shortcut,
  children,
}: {
  active: boolean;
  onClick: () => void;
  shortcut: string;
  children: React.ReactNode;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-pressed={active}
      className={cn(
        "flex items-center gap-1 rounded-md px-2 py-1 text-xs font-medium transition-colors",
        active
          ? "bg-foreground/[0.06] text-foreground"
          : "text-muted-foreground hover:text-foreground",
      )}
    >
      {children}
      <kbd className="text-muted-foreground/70 ml-0.5 font-mono text-[10px]">
        {shortcut}
      </kbd>
    </button>
  );
}

/**
 * Pending reply the autonomous agent drafted for review (CF-64 "draft"
 * mode). Hidden once a teammate has answered after it was written.
 */
function useAgentDraft(conversation: ConversationDetail, enabled: boolean) {
  const jobs = useQuery({
    queryKey: [
      "inbox",
      "ai-jobs",
      conversation.uuid,
      conversation.last_message_at,
    ],
    queryFn: () => copilotService.jobs(conversation.uuid),
    enabled,
    staleTime: 30_000,
    retry: false,
  });
  const job = jobs.data?.find((j) => j.task === "reply");
  if (!job || job.status !== "draft") return null;
  const reply = job.result.reply;
  if (typeof reply !== "string" || !reply.trim()) return null;
  if (
    conversation.last_message_direction === "out" &&
    Date.parse(conversation.last_message_at) > Date.parse(job.created_at)
  )
    return null;
  const confidence = job.result.confidence;
  return {
    uuid: job.uuid,
    text: reply.trim(),
    confidence: typeof confidence === "number" ? confidence : null,
  };
}
