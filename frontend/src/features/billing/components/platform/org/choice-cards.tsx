"use client";

import { Check } from "lucide-react";
import { RadioGroup as RadioGroupPrimitive } from "radix-ui";
import type { ComponentProps, ReactNode } from "react";

import { cn } from "@/lib/utils";

/**
 * Radio group whose options are whole cards (plan, price, owner mode,
 * timing). Arrow keys move between cards, Space selects, like any radio
 * group.
 */
export function ChoiceCards({
  className,
  ...props
}: ComponentProps<typeof RadioGroupPrimitive.Root>) {
  return (
    <RadioGroupPrimitive.Root
      className={cn("grid gap-3", className)}
      {...props}
    />
  );
}

export function ChoiceCard({
  value,
  title,
  description,
  aside,
  icon,
  children,
  className,
  disabled,
}: {
  value: string;
  title: ReactNode;
  description?: ReactNode;
  /** Right side of the title row (price, badge). */
  aside?: ReactNode;
  icon?: ReactNode;
  children?: ReactNode;
  className?: string;
  disabled?: boolean;
}) {
  return (
    <RadioGroupPrimitive.Item
      value={value}
      disabled={disabled}
      className={cn(
        "group bg-card hover:bg-muted/40 focus-visible:ring-ring/50 data-[state=checked]:border-primary data-[state=checked]:ring-primary/20 relative flex w-full items-start gap-3 rounded-xl border p-4 text-start transition-colors outline-none focus-visible:ring-[3px] disabled:cursor-not-allowed disabled:opacity-50 data-[state=checked]:ring-2",
        className,
      )}
    >
      {icon ? (
        <span className="bg-muted text-muted-foreground group-data-[state=checked]:bg-primary/10 group-data-[state=checked]:text-primary flex size-9 shrink-0 items-center justify-center rounded-lg">
          {icon}
        </span>
      ) : null}
      <span className="min-w-0 flex-1 space-y-1">
        <span className="flex flex-wrap items-baseline justify-between gap-x-3 gap-y-1">
          <span className="font-medium">{title}</span>
          {aside ? <span className="text-sm tabular-nums">{aside}</span> : null}
        </span>
        {description ? (
          <span className="text-muted-foreground block text-sm">
            {description}
          </span>
        ) : null}
        {children}
      </span>
      <span
        aria-hidden
        className="border-input group-data-[state=checked]:border-primary group-data-[state=checked]:bg-primary text-primary-foreground mt-0.5 flex size-5 shrink-0 items-center justify-center rounded-full border"
      >
        <Check className="hidden size-3.5 stroke-[3] group-data-[state=checked]:block" />
      </span>
    </RadioGroupPrimitive.Item>
  );
}
