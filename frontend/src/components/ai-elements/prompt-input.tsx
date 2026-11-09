"use client";

import { AuiIf, ComposerPrimitive } from "@assistant-ui/react";
import { ArrowUp, Loader2, Square } from "lucide-react";
import type { ComponentProps } from "react";

import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

/**
 * shadcn/ui AI Elements "Prompt Input" pattern on top of the assistant-ui
 * composer primitives: the primitives own the draft, attachments, Enter to
 * send and Shift+Enter for a new line; these components only style them.
 */
export function PromptInput({
  className,
  ...props
}: ComponentProps<typeof ComposerPrimitive.Root>) {
  return (
    <ComposerPrimitive.Root
      data-slot="prompt-input"
      className={cn("relative flex w-full flex-col", className)}
      {...props}
    />
  );
}

export function PromptInputBody({
  className,
  ...props
}: ComponentProps<typeof ComposerPrimitive.AttachmentDropzone>) {
  return (
    <ComposerPrimitive.AttachmentDropzone
      data-slot="prompt-input-body"
      className={cn(
        "border-input bg-background focus-within:border-ring focus-within:ring-ring/30 data-[dragging=true]:border-ring flex w-full cursor-text flex-col gap-1.5 rounded-xl border p-1.5 shadow-xs transition-[border-color,box-shadow] focus-within:ring-[3px] data-[dragging=true]:border-dashed",
        className,
      )}
      {...props}
    />
  );
}

export function PromptInputTextarea({
  className,
  ...props
}: ComponentProps<typeof ComposerPrimitive.Input>) {
  return (
    <ComposerPrimitive.Input
      data-slot="prompt-input-textarea"
      rows={1}
      enterKeyHint="send"
      className={cn(
        "placeholder:text-muted-foreground/70 max-h-40 min-h-9 w-full resize-none bg-transparent px-2 py-1.5 text-sm leading-6 outline-none disabled:cursor-not-allowed disabled:opacity-60",
        className,
      )}
      {...props}
    />
  );
}

export function PromptInputToolbar({
  className,
  ...props
}: ComponentProps<"div">) {
  return (
    <div
      data-slot="prompt-input-toolbar"
      className={cn("flex items-center justify-between gap-1", className)}
      {...props}
    />
  );
}

export function PromptInputTools({
  className,
  ...props
}: ComponentProps<"div">) {
  return (
    <div
      data-slot="prompt-input-tools"
      className={cn("flex items-center gap-0.5", className)}
      {...props}
    />
  );
}

export type PromptInputStatus = "ready" | "submitted" | "streaming" | "error";

/** Send while idle, stop while a run is going (assistant-ui decides which). */
export function PromptInputSubmit({
  status = "ready",
  sendLabel,
  stopLabel,
  className,
}: {
  status?: PromptInputStatus;
  sendLabel: string;
  stopLabel: string;
  className?: string;
}) {
  return (
    <>
      <AuiIf condition={(s) => !s.composer.canCancel}>
        <ComposerPrimitive.Send asChild>
          <Button
            data-slot="prompt-input-submit"
            type="submit"
            size="icon-sm"
            className={cn("rounded-full", className)}
            aria-label={sendLabel}
          >
            {status === "submitted" ? (
              <Loader2 className="size-4 animate-spin motion-reduce:animate-none" />
            ) : (
              <ArrowUp className="size-4" />
            )}
          </Button>
        </ComposerPrimitive.Send>
      </AuiIf>
      <AuiIf condition={(s) => s.composer.canCancel}>
        <ComposerPrimitive.Cancel asChild>
          <Button
            data-slot="prompt-input-stop"
            type="button"
            size="icon-sm"
            variant="secondary"
            className={cn("rounded-full", className)}
            aria-label={stopLabel}
          >
            <Square className="size-3.5 fill-current" />
          </Button>
        </ComposerPrimitive.Cancel>
      </AuiIf>
    </>
  );
}
