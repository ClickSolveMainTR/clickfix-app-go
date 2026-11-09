import { cva, type VariantProps } from "class-variance-authority";
import type { ComponentProps } from "react";

import { cn } from "@/lib/utils";

/**
 * shadcn/ui AI Elements "Marker" pattern: a small inline status line in the
 * conversation (a tool note, a hand-over notice, a "thinking…" state).
 */
export const markerVariants = cva(
  "text-muted-foreground flex items-start gap-2 text-xs [&_svg]:mt-0.5 [&_svg]:size-3.5 [&_svg]:shrink-0",
  {
    variants: {
      variant: {
        default: "",
        border: "bg-muted/40 rounded-md border px-3 py-2",
        separator:
          "before:bg-border after:bg-border items-center before:h-px before:flex-1 after:h-px after:flex-1",
      },
    },
    defaultVariants: { variant: "default" },
  },
);

export type MarkerProps = ComponentProps<"div"> &
  VariantProps<typeof markerVariants>;

export function Marker({ className, variant, ...props }: MarkerProps) {
  return (
    <div
      data-slot="marker"
      className={cn(markerVariants({ variant }), className)}
      {...props}
    />
  );
}

export function MarkerIcon({ className, ...props }: ComponentProps<"span">) {
  return (
    <span
      aria-hidden
      data-slot="marker-icon"
      className={cn("inline-flex", className)}
      {...props}
    />
  );
}

export function MarkerContent({ className, ...props }: ComponentProps<"div">) {
  return (
    <div
      data-slot="marker-content"
      className={cn("min-w-0 flex-1", className)}
      {...props}
    />
  );
}
