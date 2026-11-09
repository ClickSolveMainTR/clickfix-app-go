"use client";

import {
  ActionBarPrimitive,
  AttachmentPrimitive,
  AuiIf,
  ComposerPrimitive,
  MessagePrimitive,
  ThreadPrimitive,
  useAuiState,
  type AssistantState,
  type ThreadSuggestion,
  type ToolCallMessagePartProps,
} from "@assistant-ui/react";
import {
  ArrowDown,
  Bot,
  Check,
  Copy,
  LifeBuoy,
  Loader2,
  Paperclip,
  Pencil,
  RefreshCw,
} from "lucide-react";
import { useMemo, type ReactNode } from "react";

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
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { errorKey } from "../lib/errors";
import { PROPOSE_TOOL, type AssistantCustom } from "../lib/messages";
import { useAssistantSurface } from "../runtime/surface";

import { ActionCard, RouteLink } from "./action-card";
import { DocumentStrip } from "./document-strip";

export type AssistantThreadProps = {
  /** Shown above the starters while the thread is empty. */
  welcome?: ReactNode;
  /** Quick questions of an empty thread. */
  starters?: readonly string[];
  /** The assistant is off for the organization: composer locked. */
  disabled?: boolean;
};

/**
 * The ClickFix Asistan conversation on assistant-ui primitives, styled with
 * the shadcn/ui AI Elements patterns (Prompt Input, Suggestion, Marker,
 * Attachment; Confirmation and Tool live in the card). The drawer and any
 * narrower surface render this same thread; `compact` in the surface
 * context tightens spacing.
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
            {starters.length ? (
              <div className="grid grid-cols-2 gap-2">
                {starters.map((prompt) => (
                  <ThreadPrimitive.Suggestion
                    key={prompt}
                    prompt={prompt}
                    send
                    asChild
                  >
                    <Suggestion
                      suggestion={prompt}
                      disabled={disabled}
                      className="h-auto justify-start rounded-xl py-2 text-left whitespace-normal"
                    />
                  </ThreadPrimitive.Suggestion>
                ))}
              </div>
            ) : null}
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
        <ThreadPrimitive.ViewportFooter
          className={cn(
            "bg-background sticky bottom-0 mt-auto flex flex-col gap-2 pb-3",
          )}
        >
          <ScrollToBottom />
          <FollowupSuggestions />
          <DocumentStrip disabled={disabled} />
          <Composer disabled={disabled} />
        </ThreadPrimitive.ViewportFooter>
      </ThreadPrimitive.Viewport>
    </ThreadPrimitive.Root>
  );
}

function ThreadLoading() {
  const { t } = useLocale();
  return (
    <p role="status" className="text-muted-foreground py-4 text-sm">
      {t("common.loading")}
    </p>
  );
}

function ScrollToBottom() {
  const { t } = useLocale();
  return (
    <ThreadPrimitive.ScrollToBottom asChild>
      <Button
        variant="secondary"
        size="sm"
        className="absolute -top-10 self-center rounded-full shadow-sm disabled:invisible"
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
      <Suggestions className="-mx-1 px-1 pb-0.5">
        {suggestions.map((s, i) => (
          <ThreadPrimitive.Suggestion
            key={`${i}-${s.prompt}`}
            prompt={s.prompt}
            send
            asChild
          >
            <Suggestion suggestion={s.title ?? s.prompt} />
          </ThreadPrimitive.Suggestion>
        ))}
      </Suggestions>
    </AuiIf>
  );
}

function Composer({ disabled }: { disabled: boolean }) {
  const { t } = useLocale();
  return (
    <PromptInput>
      <PromptInputBody>
        <ComposerAttachments />
        <PromptInputTextarea
          placeholder={t("assistant.placeholder")}
          aria-label={t("assistant.placeholder")}
          maxLength={4000}
          disabled={disabled}
          autoFocus
        />
        <PromptInputToolbar>
          <PromptInputTools>
            <ComposerPrimitive.AddAttachment asChild>
              <TooltipIconButton
                tooltip={t("assistant.documents.add")}
                className="size-8 rounded-full"
                disabled={disabled}
              >
                <Paperclip className="size-4" />
              </TooltipIconButton>
            </ComposerPrimitive.AddAttachment>
          </PromptInputTools>
          <PromptInputSubmit
            sendLabel={t("assistant.send")}
            stopLabel={t("assistant.stop")}
          />
        </PromptInputToolbar>
      </PromptInputBody>
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
      <AuiIf condition={(s) => (s.message.attachments?.length ?? 0) > 0}>
        <AttachmentGroup className="justify-end">
          <MessagePrimitive.Attachments
            components={{ Attachment: () => <AttachmentChip /> }}
          />
        </AttachmentGroup>
      </AuiIf>
      <div className="bg-primary text-primary-foreground max-w-[92%] min-w-0 rounded-2xl px-4 py-2.5 text-sm break-words">
        <MessagePrimitive.Parts components={{ Text: UserText }} />
      </div>
      <ActionBarPrimitive.Root
        hideWhenRunning
        autohide="not-last"
        className="text-muted-foreground flex gap-1"
      >
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
      <ComposerPrimitive.Root className="bg-muted flex w-[92%] flex-col gap-2 rounded-2xl p-2">
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
  Text: MarkdownText,
  tools: { by_name: { [PROPOSE_TOOL]: ActionCard }, Fallback: ToolFallback },
};

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
      <MessagePrimitive.Root data-role="tool">
        <Marker variant="border">
          <MarkerIcon>
            <LifeBuoy />
          </MarkerIcon>
          <MarkerContent>
            <MessagePrimitive.Parts components={PART_COMPONENTS} />
            <ToolNoteLinks />
          </MarkerContent>
        </Marker>
      </MessagePrimitive.Root>
    );
  }
  return (
    <MessagePrimitive.Root
      data-role="assistant"
      className="flex flex-col items-start gap-1"
    >
      <div className="flex w-full items-start gap-2">
        <span className="bg-primary/10 text-primary mt-1 flex size-6 shrink-0 items-center justify-center rounded-md">
          <Bot className="size-3.5" />
        </span>
        <div className="min-w-0 flex-1 text-sm">
          {running && empty ? (
            <Marker role="status" className="py-1.5">
              <MarkerIcon>
                <Loader2 className="animate-spin motion-reduce:animate-none" />
              </MarkerIcon>
              <MarkerContent>
                {statusText || t("assistant.thinking")}
              </MarkerContent>
            </Marker>
          ) : null}
          <MessagePrimitive.Parts components={PART_COMPONENTS} />
          {running && !empty && statusText ? (
            <Marker role="status" className="py-1">
              <MarkerIcon>
                <Loader2 className="animate-spin motion-reduce:animate-none" />
              </MarkerIcon>
              <MarkerContent>{statusText}</MarkerContent>
            </Marker>
          ) : null}
          {failed ? (
            <p role="alert" className="text-destructive mt-1 text-xs">
              {t(errorKey(status.error))}
            </p>
          ) : null}
          {cancelled || custom?.interrupted ? (
            <p className="text-muted-foreground mt-1 text-xs">
              {t("assistant.interrupted")}
            </p>
          ) : null}
        </div>
      </div>
      <ActionBarPrimitive.Root
        hideWhenRunning
        autohide="not-last"
        className="text-muted-foreground ml-8 flex gap-1"
      >
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
        <ActionBarPrimitive.Reload asChild>
          <TooltipIconButton tooltip={t("assistant.regenerate")}>
            <RefreshCw />
          </TooltipIconButton>
        </ActionBarPrimitive.Reload>
      </ActionBarPrimitive.Root>
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
