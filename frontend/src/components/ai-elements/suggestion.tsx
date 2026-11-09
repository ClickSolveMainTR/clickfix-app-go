"use client";

import type { ComponentProps } from "react";

import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

/**
 * shadcn/ui AI Elements "Suggestion" pattern: a horizontal row of quick
 * replies. `Suggestion` calls `onClick` with the suggestion text.
 */
export function Suggestions({ className, ...props }: ComponentProps<"div">) {
  return (
    <div
      data-slot="suggestions"
      className={cn(
        "flex w-full [scrollbar-width:none] gap-2 overflow-x-auto [&::-webkit-scrollbar]:hidden",
        className,
      )}
      {...props}
    />
  );
}

export type SuggestionProps = Omit<ComponentProps<typeof Button>, "onClick"> & {
  suggestion: string;
  onClick?: (suggestion: string) => void;
};

export function Suggestion({
  suggestion,
  onClick,
  className,
  variant = "outline",
  size = "sm",
  children,
  ...props
}: SuggestionProps) {
  return (
    <Button
      data-slot="suggestion"
      type="button"
      variant={variant}
      size={size}
      className={cn("shrink-0 rounded-full", className)}
      onClick={() => onClick?.(suggestion)}
      {...props}
    >
      {children ?? suggestion}
    </Button>
  );
}
