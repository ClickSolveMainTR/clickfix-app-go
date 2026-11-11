"use client";

import {
  ActionBarPrimitive,
  AttachmentPrimitive,
  AuiIf,
  ComposerPrimitive,
  MessagePrimitive,
  ThreadPrimitive,
  useAui,
  useAuiState,
  type AssistantState,
  type ThreadSuggestion,
  type ToolCallMessagePartProps,
} from "@assistant-ui/react";
import {
  ArrowDown,
  Check,
  Copy,
  Loader2,
  Paperclip,
  Pencil,
  RefreshCw,
  X,
} from "lucide-react";
import {
  useEffect,
  useMemo,
  useRef,
  useSyncExternalStore,
  type KeyboardEvent,
  type ReactNode,
} from "react";

import {
  Attachment,
  AttachmentAction,
  AttachmentGroup,
  AttachmentTitle,
} from "@/components/ai-elements/attachment";
import {
  Marker,
  MarkerContent,
  MarkerIcon,
} from "@/components/ai-elements/marker";
import {
  PromptInput,
  PromptInputBody,
  PromptInputSubmit,
  PromptInputTextarea,
  PromptInputToolbar,
  PromptInputTools,
} from "@/components/ai-elements/prompt-input";
import { Suggestion, Suggestions } from "@/components/ai-elements/suggestion";
import { MarkdownText } from "@/components/assistant-ui/markdown-text";
import { TooltipIconButton } from "@/components/assistant-ui/tooltip-icon-button";
import { AppMark } from "@/components/brand";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { cn } from "@/lib/utils";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

import { errorKey, errorKeyForCode } from "../lib/errors";
import { PROPOSE_TOOL, type AssistantCustom } from "../lib/messages";
import { shouldSendQueued } from "../lib/queue";
import { messageTimePattern, showCharCount } from "../lib/time";
import { DOCUMENT_ACCEPT } from "../runtime/attachments";
import { useAssistantSurface } from "../runtime/surface";

import { ActionCard, RouteLink } from "./action-card";
import { AssistantAvatar } from "./assistant-avatar";
import { DocumentStrip } from "./document-strip";

const COMPOSER_MAX = 4000;

/** A quick question of the empty thread, with an optional glyph. */
export type AssistantStarter = { prompt: string; icon?: ReactNode };

export type AssistantThreadProps = {
  /** Shown above the starters while the thread is empty. */
  welcome?: ReactNode;
  /** Quick questions of an empty thread. */
  starters?: readonly (string | AssistantStarter)[];
  /** The assistant is off for the organization: composer locked. */
  disabled?: boolean;
};

/**
 * The ClickFix Asistan conversation on assistant-ui primitives, styled with
 * the shadcn/ui AI Elements patterns (Prompt Input, Suggestion, Marker,
 * Attachment; Confirmation and Tool live in the card). The drawer and any
 * narrower surface render this same thread; `compact` in the surface
 * context tightens spacing.
 *
 * Look (CF-210): the user's words sit right in the brand colour, the
 * assistant's answers sit left on a neutral card next to the ClickFix mark;
 * every bubble carries its time under it in a fixed 28px action row.
 */
export function AssistantThread({
  welcome,
  starters = [],
  disabled = false,
}: AssistantThreadProps) {
  const { compact } = useAssistantSurface();
  return (
    <ThreadPrimitive.Root className="flex h-full min-h-0 flex-col">
      <ThreadPrimitive.Viewport
        className={cn(
          "relative flex flex-1 flex-col overflow-x-hidden overflow-y-auto scroll-smooth",
          compact ? "px-3 pt-3" : "px-4 pt-4",
        )}
      >
        <AuiIf
          condition={(s) =>
            s.thread.messages.length === 0 && !s.thread.isLoading
          }
        >
          <div className="space-y-3">
            {welcome}
            <Starters starters={starters} disabled={disabled} />
          </div>
        </AuiIf>
        <AuiIf
          condition={(s) =>
            s.thread.messages.length === 0 && s.thread.isLoading
          }
        >
          <ThreadLoading />
        </AuiIf>
        <div className={cn("flex flex-col pb-4", compact ? "gap-3" : "gap-4")}>
          <ThreadPrimitive.Messages
            components={{ UserMessage, AssistantMessage, EditComposer }}
          />
        </div>
        <ThreadPrimitive.ViewportFooter className="bg-background sticky bottom-0 mt-auto flex flex-col gap-2 pb-3">
          <ScrollToBottom />
          <FollowupSuggestions />
          <DocumentStrip disabled={disabled} />
          <Composer disabled={disabled} />
        </ThreadPrimitive.ViewportFooter>
      </ThreadPrimitive.Viewport>
    </ThreadPrimitive.Root>
  );
}

/**
 * The welcome card of an empty thread: the mark, a short introduction. The
 * panel adds the page state card under it.
 */
export function AssistantWelcome({
  title,
  body,
  eyebrow,
}: {
  title: string;
  body: string;
  eyebrow?: string;
}) {
  return (
    <div
      data-slot="assistant-welcome"
      className="bg-card flex gap-3 rounded-2xl border p-4 text-sm shadow-xs"
    >
      <AssistantAvatar size="lg" />
      <div className="min-w-0 space-y-1">
        {eyebrow ? (
          <p className="font-accent text-muted-foreground flex items-center gap-2 text-[11px] font-semibold tracking-[0.16em] uppercase">
            <span aria-hidden className="signal-dot size-1.5!" />
            <span className="truncate">{eyebrow}</span>
          </p>
        ) : null}
        <p className="font-display font-semibold tracking-tight">{title}</p>
        <p className="text-muted-foreground text-pretty">{body}</p>
      </div>
    </div>
  );
}

function Starters({
  starters,
  disabled,
}: {
  starters: readonly (string | AssistantStarter)[];
  disabled: boolean;
}) {
  const { t } = useLocale();
  const { compact } = useAssistantSurface();
  if (!starters.length) return null;
  return (
    <div className="space-y-2">
      <p className="text-muted-foreground px-1 text-xs">
        {t("assistant.welcome.starters")}
      </p>
      <div
        className={cn("grid gap-2", compact ? "grid-cols-1" : "grid-cols-2")}
      >
        {starters.map((s) => {
          const item = typeof s === "string" ? { prompt: s } : s;
          return (
            <ThreadPrimitive.Suggestion
              key={item.prompt}
              prompt={item.prompt}
              send
              asChild
            >
              <Suggestion
                suggestion={item.prompt}
                disabled={disabled}
                className="bg-card hover:border-signal/50 hover:bg-card h-auto items-start justify-start gap-2 rounded-xl px-3 py-2.5 text-left font-normal whitespace-normal shadow-xs"
              >
                {item.icon ? (
                  <span
                    aria-hidden
                    className="text-signal-text mt-0.5 shrink-0 [&_svg]:size-4"
                  >
                    {item.icon}
                  </span>
                ) : null}
                <span className="min-w-0">{item.prompt}</span>
              </Suggestion>
            </ThreadPrimitive.Suggestion>
          );
        })}
      </div>
    </div>
  );
}

/** Two ghost bubbles while a past chat loads. */
function ThreadLoading() {
  const { t } = useLocale();
  return (
    <div
      role="status"
      aria-label={t("assistant.loading")}
      className="space-y-4"
    >
      <div className="flex justify-end">
        <Skeleton className="h-9 w-1/2 rounded-2xl rounded-br-md" />
      </div>
      <div className="flex gap-2.5">
        <AssistantAvatar size="sm" />
        <div className="flex-1 space-y-2">
          <Skeleton className="h-16 w-4/5 rounded-2xl rounded-tl-md" />
          <Skeleton className="h-3 w-24" />
        </div>
      </div>
      <span className="sr-only">{t("assistant.loading")}</span>
    </div>
  );
}

function ScrollToBottom() {
  const { t } = useLocale();
  return (
    <ThreadPrimitive.ScrollToBottom asChild>
      <Button
        variant="secondary"
        size="sm"
        className="absolute -top-10 self-center rounded-full border shadow-sm disabled:invisible"
      >
        <ArrowDown className="size-4" />
        {t("assistant.latest")}
      </Button>
    </ThreadPrimitive.ScrollToBottom>
  );
}

/**
 * Quick replies the backend saved with the last answer. The suggestion
 * adapter fills `thread.suggestions` once a run settles; a run paused on a
 * confirmation card never settles, so the replies saved on that message
 * are read directly.
 */
/** The replies saved on the last settled assistant message (stable reference). */
const selectSavedReplies = (s: AssistantState) => {
  const last = s.thread.messages.at(-1);
  if (!last || last.role !== "assistant" || last.status?.type === "running")
    return undefined;
  return (last.metadata.custom as AssistantCustom | undefined)?.suggestions;
};

function FollowupSuggestions() {
  const { t } = useLocale();
  const fromAdapter = useAuiState((s) => s.thread.suggestions);
  const saved = useAuiState(selectSavedReplies);
  const suggestions = useMemo<readonly ThreadSuggestion[]>(
    () =>
      fromAdapter.length
        ? fromAdapter
        : (saved ?? []).map((prompt) => ({ prompt })),
    [fromAdapter, saved],
  );
  if (!suggestions.length) return null;
  return (
    <AuiIf condition={(s) => !s.thread.isEmpty && !s.thread.isRunning}>
      <Suggestions
        aria-label={t("assistant.suggestions.title")}
        className="-mx-1 px-1 pb-0.5"
      >
        {suggestions.map((s, i) => (
          <ThreadPrimitive.Suggestion
            key={`${i}-${s.prompt}`}
            prompt={s.prompt}
            send
            asChild
          >
            <Suggestion
              suggestion={s.title ?? s.prompt}
              className="bg-card hover:border-signal/50"
            />
          </ThreadPrimitive.Suggestion>
        ))}
      </Suggestions>
    </AuiIf>
  );
}

/**
 * The composer. Two CF-207 (#150) rules live here:
 * - The paperclip is a real `input[type=file]` in the DOM (assistant-ui's
 *   AddAttachment builds a hidden input on click, which left the picker
 *   closed on the live panel); the picked files go to the attachment
 *   adapter like before.
 * - Enter while a reply is streaming does not pile a second question into
 *   the same box: the text is queued and sent as its own message once the
 *   run settles. The queue is shown under the box and can be dropped.
 */
function Composer({ disabled }: { disabled: boolean }) {
  const { t } = useLocale();
  const aui = useAui();
  const { queue } = useAssistantSurface();
  const fileInput = useRef<HTMLInputElement>(null);
  const queued = useSyncExternalStore(
    (listener) => queue.subscribe(listener),
    () => queue.size,
    () => 0,
  );
  const isRunning = useAuiState((s) => s.thread.isRunning);
  const length = useAuiState((s) => s.composer.text.length);
  const wasRunning = useRef(isRunning);
  useEffect(() => {
    if (shouldSendQueued(wasRunning.current, isRunning, queue.size)) {
      // Stop drops the queue: the user interrupted on purpose and would not
      // expect the next question to go out on its own.
      const last = aui.thread().getState().messages.at(-1);
      if (
        last?.status?.type === "incomplete" &&
        last.status.reason === "cancelled"
      ) {
        queue.clear();
      } else {
        const next = queue.next();
        if (next) void aui.thread().append(next);
      }
    }
    wasRunning.current = isRunning;
  }, [aui, isRunning, queue, queued]);

  const onKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key !== "Enter" || e.shiftKey || e.nativeEvent.isComposing) return;
    if (!aui.thread().getState().isRunning) return;
    e.preventDefault();
    const text = aui.composer().getState().text;
    if (queue.push(text)) aui.composer().setText("");
  };

  const addFiles = (files: FileList | null) => {
    for (const file of Array.from(files ?? []))
      void aui.composer().addAttachment(file);
    if (fileInput.current) fileInput.current.value = "";
  };

  const placeholder = disabled
    ? t("assistant.composer.locked")
    : t("assistant.placeholder");

  return (
    <PromptInput>
      <PromptInputBody className={cn(disabled && "bg-muted/40")}>
        <ComposerAttachments />
        <PromptInputTextarea
          placeholder={placeholder}
          aria-label={t("assistant.placeholder")}
          maxLength={COMPOSER_MAX}
          disabled={disabled}
          onKeyDown={onKeyDown}
          autoFocus
        />
        <PromptInputToolbar>
          <PromptInputTools>
            <input
              ref={fileInput}
              type="file"
              accept={DOCUMENT_ACCEPT}
              multiple
              hidden
              tabIndex={-1}
              aria-hidden="true"
              data-testid="assistant-file-input"
              onChange={(e) => addFiles(e.target.files)}
            />
            <TooltipIconButton
              type="button"
              tooltip={t("assistant.documents.file")}
              className="text-muted-foreground hover:text-foreground size-8 rounded-full"
              disabled={disabled}
              onClick={() => fileInput.current?.click()}
            >
              <Paperclip className="size-4" />
            </TooltipIconButton>
          </PromptInputTools>
          <div className="flex items-center gap-2">
            {showCharCount(length, COMPOSER_MAX) ? (
              <span
                role="status"
                className={cn(
                  "text-[11px] tabular-nums",
                  length >= COMPOSER_MAX
                    ? "text-destructive"
                    : "text-muted-foreground",
                )}
              >
                {t("assistant.composer.count", {
                  count: length,
                  max: COMPOSER_MAX,
                })}
              </span>
            ) : null}
            <PromptInputSubmit
              sendLabel={t("assistant.send")}
              stopLabel={t("assistant.stop")}
            />
          </div>
        </PromptInputToolbar>
      </PromptInputBody>
      <div className="text-muted-foreground flex min-h-5 items-center justify-between gap-2 px-2 pt-1 text-[11px]">
        {queued > 0 ? (
          <>
            <span role="status" className="truncate">
              {t("assistant.queued", { count: queued })}
            </span>
            <button
              type="button"
              className="hover:text-foreground focus-visible:ring-ring/50 inline-flex shrink-0 items-center gap-1 rounded-sm outline-none focus-visible:ring-2"
              onClick={() => queue.clear()}
            >
              <X className="size-3" />
              {t("assistant.queued_drop")}
            </button>
          </>
        ) : (
          <span className="hidden truncate sm:inline">
            {t("assistant.composer.hint")}
          </span>
        )}
      </div>
    </PromptInput>
  );
}

function ComposerAttachments() {
  const { t } = useLocale();
  return (
    <AuiIf condition={(s) => s.composer.attachments.length > 0}>
      <AttachmentGroup className="px-1">
        <ComposerPrimitive.Attachments
          components={{
            Attachment: () => (
              <AttachmentChip
                removeLabel={(name) =>
                  t("assistant.documents.remove", { name })
                }
              />
            ),
          }}
        />
      </AttachmentGroup>
    </AuiIf>
  );
}

function AttachmentChip({
  removeLabel,
}: {
  removeLabel?: (name: string) => string;
}) {
  const name = useAuiState((s) => s.attachment.name);
  const status = useAuiState((s) => s.attachment.status);
  const message = status.type === "incomplete" ? status.message : undefined;
  const state =
    status.type === "incomplete"
      ? "error"
      : status.type === "running"
        ? "uploading"
        : "done";
  return (
    <AttachmentPrimitive.Root asChild>
      <Attachment state={state} title={message ?? name}>
        <AttachmentTitle>{name}</AttachmentTitle>
        {removeLabel ? (
          <AttachmentPrimitive.Remove asChild>
            <AttachmentAction label={removeLabel(name)} />
          </AttachmentPrimitive.Remove>
        ) : null}
      </Attachment>
    </AttachmentPrimitive.Root>
  );
}

/** When the message was sent, in the action row under the bubble. */
function MessageTime() {
  const { locale } = useLocale();
  const createdAt = useAuiState((s) => s.message.createdAt);
  if (!createdAt) return null;
  return (
    <time
      dateTime={createdAt.toISOString()}
      className="text-muted-foreground px-1 text-[11px] tabular-nums"
    >
      {datetime(createdAt, messageTimePattern(createdAt), locale)}
    </time>
  );
}

function CopyAction() {
  const { t } = useLocale();
  return (
    <ActionBarPrimitive.Copy asChild>
      <TooltipIconButton tooltip={t("assistant.copy")}>
        <AuiIf condition={(s) => s.message.isCopied}>
          <Check />
        </AuiIf>
        <AuiIf condition={(s) => !s.message.isCopied}>
          <Copy />
        </AuiIf>
      </TooltipIconButton>
    </ActionBarPrimitive.Copy>
  );
}

function UserText({ text }: { text: string }) {
  return <p className="whitespace-pre-wrap">{text}</p>;
}

function UserMessage() {
  const { t } = useLocale();
  return (
    <MessagePrimitive.Root
      data-role="user"
      className="flex flex-col items-end gap-1"
    >
      <span className="sr-only">{t("assistant.you")}</span>
      <AuiIf condition={(s) => (s.message.attachments?.length ?? 0) > 0}>
        <AttachmentGroup className="justify-end">
          <MessagePrimitive.Attachments
            components={{ Attachment: () => <AttachmentChip /> }}
          />
        </AttachmentGroup>
      </AuiIf>
      <div className="bg-primary text-primary-foreground max-w-[85%] min-w-0 rounded-2xl rounded-br-md px-3.5 py-2.5 text-sm break-words shadow-xs">
        <MessagePrimitive.Parts components={{ Text: UserText }} />
      </div>
      <ActionBarPrimitive.Root className="text-muted-foreground flex h-7 items-center gap-1">
        <MessageTime />
        <CopyAction />
        <ActionBarPrimitive.Edit asChild>
          <TooltipIconButton tooltip={t("assistant.edit")}>
            <Pencil />
          </TooltipIconButton>
        </ActionBarPrimitive.Edit>
      </ActionBarPrimitive.Root>
    </MessagePrimitive.Root>
  );
}

function EditComposer() {
  const { t } = useLocale();
  return (
    <MessagePrimitive.Root data-role="user" className="flex justify-end">
      <ComposerPrimitive.Root className="bg-card focus-within:border-ring focus-within:ring-ring/30 flex w-[85%] flex-col gap-2 rounded-2xl border p-2 shadow-xs focus-within:ring-[3px]">
        <ComposerPrimitive.Input
          className="min-h-9 w-full resize-none bg-transparent px-2 py-1 text-sm outline-none"
          aria-label={t("assistant.edit")}
          autoFocus
        />
        <div className="flex justify-end gap-1">
          <ComposerPrimitive.Cancel asChild>
            <Button size="sm" variant="ghost">
              {t("common.cancel")}
            </Button>
          </ComposerPrimitive.Cancel>
          <ComposerPrimitive.Send asChild>
            <Button size="sm">{t("assistant.send")}</Button>
          </ComposerPrimitive.Send>
        </div>
      </ComposerPrimitive.Root>
    </MessagePrimitive.Root>
  );
}

/** The assistant's words on a neutral card; cards of tool calls stand alone. */
function AssistantText() {
  return (
    <div
      data-slot="assistant-bubble"
      className="bg-card text-card-foreground w-fit max-w-full min-w-0 rounded-2xl rounded-tl-md border px-3.5 py-2.5 text-sm break-words shadow-xs"
    >
      <MarkdownText />
    </div>
  );
}

function ToolFallback(props: ToolCallMessagePartProps) {
  return (
    <Marker variant="border">
      <MarkerContent>{props.toolName}</MarkerContent>
    </Marker>
  );
}

const NoEmpty = () => null;

const PART_COMPONENTS = {
  Empty: NoEmpty,
  Text: AssistantText,
  tools: { by_name: { [PROPOSE_TOOL]: ActionCard }, Fallback: ToolFallback },
};

const NOTE_PART_COMPONENTS = {
  Empty: NoEmpty,
  Text: MarkdownText,
  tools: { by_name: { [PROPOSE_TOOL]: ActionCard }, Fallback: ToolFallback },
};

/** Three soft dots: the reply is on its way (still for reduced motion). */
function TypingDots() {
  return (
    <span aria-hidden className="inline-flex items-center gap-1">
      {[0, 1, 2].map((i) => (
        <span
          key={i}
          className="bg-muted-foreground/70 size-1.5 animate-pulse rounded-full motion-reduce:animate-none"
          style={{ animationDelay: `${i * 160}ms` }}
        />
      ))}
    </span>
  );
}

function AssistantMessage() {
  const { t } = useLocale();
  const custom = useAuiState(
    (s) => s.message.metadata.custom as AssistantCustom | undefined,
  );
  const status = useAuiState((s) => s.message.status);
  const running = status?.type === "running";
  const statusText = useAuiState(
    (s) => (s.message.metadata.custom as { status?: string }).status,
  );
  const empty = useAuiState(
    (s) =>
      !s.message.content.some(
        (p) => (p.type === "text" && p.text) || p.type === "tool-call",
      ),
  );
  const failed = status?.type === "incomplete" && status.reason === "error";
  const cancelled =
    status?.type === "incomplete" && status.reason === "cancelled";

  if (custom?.role === "tool") {
    return (
      <MessagePrimitive.Root data-role="tool" className="pl-9">
        <Marker variant="border" aria-label={t("assistant.tool_note")}>
          <MarkerIcon>
            <AppMark glyph="cursor" title="" className="text-signal-text" />
          </MarkerIcon>
          <MarkerContent>
            <MessagePrimitive.Parts components={NOTE_PART_COMPONENTS} />
            <ToolNoteLinks />
          </MarkerContent>
        </Marker>
      </MessagePrimitive.Root>
    );
  }
  return (
    <MessagePrimitive.Root
      data-role="assistant"
      className="flex items-start gap-2.5"
    >
      <AssistantAvatar size="sm" className="mt-0.5" />
      <div className="flex min-w-0 flex-1 flex-col items-start gap-1">
        <span className="sr-only">{t("assistant.assistant_name")}</span>
        <div className="flex w-full min-w-0 flex-col gap-2 text-sm">
          {running && empty ? (
            <div
              role="status"
              className="bg-card text-muted-foreground flex w-fit items-center gap-2 rounded-2xl rounded-tl-md border px-3.5 py-2.5 text-xs shadow-xs"
            >
              <TypingDots />
              {statusText || t("assistant.typing")}
            </div>
          ) : null}
          <MessagePrimitive.Parts components={PART_COMPONENTS} />
          {running && !empty ? (
            <Marker role="status" className="px-1">
              <MarkerIcon>
                {statusText ? (
                  <Loader2 className="animate-spin motion-reduce:animate-none" />
                ) : null}
              </MarkerIcon>
              <MarkerContent>
                {statusText ? statusText : <TypingDots />}
              </MarkerContent>
            </Marker>
          ) : null}
          {failed ? (
            <p
              role="alert"
              className="bg-destructive/5 text-destructive border-destructive/20 rounded-lg border px-3 py-2 text-xs"
            >
              {t(errorKey(status.error))}
            </p>
          ) : null}
          {/* History tells a failed turn (error code saved) from one the
              user stopped; a live failure already shows the alert above. */}
          {!failed && custom?.errorCode ? (
            <p className="bg-destructive/5 text-destructive border-destructive/20 rounded-lg border px-3 py-2 text-xs">
              {t("assistant.failed_turn")}{" "}
              {t(errorKeyForCode(custom.errorCode))}
            </p>
          ) : cancelled || custom?.interrupted ? (
            <Marker variant="separator" className="px-1">
              <MarkerContent className="flex-none">
                {t("assistant.interrupted")}
              </MarkerContent>
            </Marker>
          ) : null}
        </div>
        <ActionBarPrimitive.Root className="text-muted-foreground flex h-7 items-center gap-1">
          <MessageTime />
          <CopyAction />
          <ActionBarPrimitive.Reload asChild>
            <TooltipIconButton tooltip={t("assistant.regenerate")}>
              <RefreshCw />
            </TooltipIconButton>
          </ActionBarPrimitive.Reload>
        </ActionBarPrimitive.Root>
      </div>
    </MessagePrimitive.Root>
  );
}

/** "Open the page" links of a tool note (the API's actions carry them). */
function ToolNoteLinks() {
  const links = useAuiState((s) =>
    s.message.content.flatMap((p) => {
      if (p.type !== "tool-call") return [];
      const a = p.artifact as {
        id: string;
        focus?: { route: string; target?: string | null } | null;
        route?: string | null;
      };
      if (a.focus)
        return [{ id: a.id, route: a.focus.route, target: a.focus.target }];
      if (a.route) return [{ id: a.id, route: a.route, target: null }];
      return [];
    }),
  );
  return (
    <>
      {links.map((l) => (
        <RouteLink key={l.id} route={l.route} target={l.target} />
      ))}
    </>
  );
}
