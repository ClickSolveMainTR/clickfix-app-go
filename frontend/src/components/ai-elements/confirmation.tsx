"use client";

import { createContext, useContext, type ComponentProps } from "react";

import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

/**
 * shadcn/ui AI Elements "Confirmation" pattern: a card that asks the user to
 * approve or deny a tool call. `state` follows the approval of the call:
 * requested → accepted | rejected | cancelled.
 */
export type ConfirmationState =
  "requested" | "accepted" | "rejected" | "cancelled";

const ConfirmationContext = createContext<ConfirmationState>("requested");

export type ConfirmationProps = ComponentProps<"div"> & {
  state: ConfirmationState;
  destructive?: boolean;
};

export function Confirmation({
  state,
  destructive,
  className,
  ...props
}: ConfirmationProps) {
  return (
    <ConfirmationContext.Provider value={state}>
      <div
        data-slot="confirmation"
        data-state={state}
        className={cn(
          "bg-card text-card-foreground flex w-full flex-col gap-3 rounded-xl border p-3 text-sm shadow-xs",
          state === "requested" && "border-warning/40",
          destructive && state === "requested" && "border-destructive/40",
          className,
        )}
        {...props}
      />
    </ConfirmationContext.Provider>
  );
}

export function ConfirmationTitle({
  className,
  ...props
}: ComponentProps<"div">) {
  return (
    <div
      data-slot="confirmation-title"
      className={cn("flex items-start justify-between gap-2", className)}
      {...props}
    />
  );
}

export function ConfirmationRequest(props: ComponentProps<"div">) {
  const state = useContext(ConfirmationContext);
  if (state !== "requested") return null;
  return <div data-slot="confirmation-request" {...props} />;
}

export function ConfirmationAccepted(props: ComponentProps<"div">) {
  const state = useContext(ConfirmationContext);
  if (state !== "accepted") return null;
  return <div data-slot="confirmation-accepted" {...props} />;
}

export function ConfirmationRejected(props: ComponentProps<"div">) {
  const state = useContext(ConfirmationContext);
  if (state !== "rejected" && state !== "cancelled") return null;
  return <div data-slot="confirmation-rejected" {...props} />;
}

export function ConfirmationActions({
  className,
  ...props
}: ComponentProps<"div">) {
  const state = useContext(ConfirmationContext);
  if (state !== "requested") return null;
  return (
    <div
      data-slot="confirmation-actions"
      className={cn("flex flex-wrap items-center gap-2", className)}
      {...props}
    />
  );
}

export function ConfirmationAction({
  size = "sm",
  ...props
}: ComponentProps<typeof Button>) {
  return <Button data-slot="confirmation-action" size={size} {...props} />;
}
