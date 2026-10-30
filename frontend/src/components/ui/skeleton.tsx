import { cn } from "@/lib/utils";

/** Placeholder block with a soft sweep (still under reduced motion). */
export function Skeleton({
  className,
  ...props
}: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      data-slot="skeleton"
      aria-hidden
      className={cn("skeleton-sweep bg-muted rounded-md", className)}
      {...props}
    />
  );
}
