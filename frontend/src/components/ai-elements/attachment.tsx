"use client";

import { AlertCircle, FileText, Link2, Loader2, X } from "lucide-react";
import type { ComponentProps, ReactNode } from "react";

import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

/**
 * shadcn/ui AI Elements "Attachment" pattern as compact chips: a group row,
 * one chip per file or link with its state (uploading / error / done) and an
 * optional remove action.
 */
export type AttachmentState =
  "idle" | "uploading" | "processing" | "error" | "done";

export function AttachmentGroup({ className, ...props }: ComponentProps<"ul">) {
  return (
    <ul
      data-slot="attachment-group"
      className={cn("flex flex-wrap items-center gap-1", className)}
      {...props}
    />
  );
}

export function Attachment({
  state = "done",
  kind = "file",
  className,
  children,
  ...props
}: ComponentProps<"li"> & {
  state?: AttachmentState;
  kind?: "file" | "link";
}) {
  return (
    <li
      data-slot="attachment"
      data-state={state}
      className={cn(
        "bg-muted flex max-w-full items-center gap-1 rounded-full px-2 py-0.5 text-xs",
        state === "error" && "text-destructive bg-destructive/10",
        className,
      )}
      {...props}
    >
      <AttachmentMedia state={state} kind={kind} />
      {children}
    </li>
  );
}

export function AttachmentMedia({
  state,
  kind,
}: {
  state: AttachmentState;
  kind: "file" | "link";
}) {
  if (state === "uploading" || state === "processing")
    return (
      <Loader2 className="size-3 shrink-0 animate-spin motion-reduce:animate-none" />
    );
  if (state === "error") return <AlertCircle className="size-3 shrink-0" />;
  return kind === "link" ? (
    <Link2 className="size-3 shrink-0" />
  ) : (
    <FileText className="size-3 shrink-0" />
  );
}

export function AttachmentTitle({
  className,
  ...props
}: ComponentProps<"span">) {
  return (
    <span
      data-slot="attachment-title"
      className={cn("truncate", className)}
      {...props}
    />
  );
}

export function AttachmentAction({
  label,
  children,
  className,
  ...props
}: ComponentProps<typeof Button> & { label: string; children?: ReactNode }) {
  return (
    <Button
      data-slot="attachment-action"
      type="button"
      variant="ghost"
      size="icon-xs"
      className={cn(
        "text-muted-foreground hover:text-foreground size-4 rounded-full",
        className,
      )}
      aria-label={label}
      {...props}
    >
      {children ?? <X className="size-3" />}
    </Button>
  );
}
