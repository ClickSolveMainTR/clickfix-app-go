"use client";

import {
  CheckCircle2,
  ChevronDown,
  CircleDashed,
  Loader2,
  XCircle,
} from "lucide-react";
import type { ComponentProps, ReactNode } from "react";

import { Badge } from "@/components/ui/badge";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import { cn } from "@/lib/utils";

/**
 * shadcn/ui AI Elements "Tool" pattern: a collapsible record of one tool call
 * (header with status badge, input, output). The states mirror the pattern's
 * `input-available | approval-requested | output-available | output-error |
 * output-denied`.
 */
export type ToolState =
  | "input-streaming"
  | "input-available"
  | "approval-requested"
  | "output-available"
  | "output-error"
  | "output-denied";

export function Tool({
  className,
  ...props
}: ComponentProps<typeof Collapsible>) {
  return (
    <Collapsible
      data-slot="tool"
      className={cn("bg-card w-full rounded-xl border text-sm", className)}
      {...props}
    />
  );
}

export function getStatusBadge(state: ToolState, label: string) {
  const icon =
    state === "input-streaming" ? (
      <Loader2 className="size-3 animate-spin motion-reduce:animate-none" />
    ) : state === "output-available" ? (
      <CheckCircle2 className="size-3" />
    ) : state === "output-error" || state === "output-denied" ? (
      <XCircle className="size-3" />
    ) : (
      <CircleDashed className="size-3" />
    );
  return (
    <Badge
      variant={
        state === "output-error"
          ? "danger"
          : state === "output-available"
            ? "default"
            : "secondary"
      }
      className="gap-1 rounded-full"
    >
      {icon}
      {label}
    </Badge>
  );
}

export function ToolHeader({
  title,
  state,
  statusLabel,
  className,
  ...props
}: ComponentProps<typeof CollapsibleTrigger> & {
  title: ReactNode;
  state: ToolState;
  statusLabel: string;
}) {
  return (
    <CollapsibleTrigger
      data-slot="tool-header"
      className={cn(
        "group flex w-full items-center gap-2 px-3 py-2 text-left",
        className,
      )}
      {...props}
    >
      <span className="min-w-0 flex-1 truncate font-medium">{title}</span>
      {getStatusBadge(state, statusLabel)}
      <ChevronDown className="text-muted-foreground size-4 shrink-0 transition-transform group-data-[state=open]:rotate-180" />
    </CollapsibleTrigger>
  );
}

export function ToolContent({
  className,
  ...props
}: ComponentProps<typeof CollapsibleContent>) {
  return (
    <CollapsibleContent
      data-slot="tool-content"
      className={cn("space-y-3 border-t px-3 py-2", className)}
      {...props}
    />
  );
}

export function ToolInput({
  input,
  label,
  className,
}: {
  input: Record<string, unknown>;
  label: string;
  className?: string;
}) {
  const entries = Object.entries(input).filter(
    ([, v]) => v !== "" && v !== null && v !== undefined,
  );
  if (!entries.length) return null;
  return (
    <div data-slot="tool-input" className={cn("space-y-1", className)}>
      <p className="text-muted-foreground text-xs">{label}</p>
      <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-0.5 text-xs">
        {entries.map(([k, v]) => (
          <div key={k} className="contents">
            <dt className="text-muted-foreground">{k}</dt>
            <dd className="min-w-0 truncate">
              {typeof v === "object" ? JSON.stringify(v) : String(v)}
            </dd>
          </div>
        ))}
      </dl>
    </div>
  );
}

export function ToolOutput({
  output,
  errorText,
  className,
}: {
  output?: ReactNode;
  errorText?: string | null;
  className?: string;
}) {
  if (!output && !errorText) return null;
  return (
    <div
      data-slot="tool-output"
      className={cn(
        "rounded-md border p-2 text-xs",
        errorText
          ? "border-destructive/30 bg-destructive/5 text-destructive"
          : "bg-muted/40",
        className,
      )}
    >
      {errorText ?? output}
    </div>
  );
}
