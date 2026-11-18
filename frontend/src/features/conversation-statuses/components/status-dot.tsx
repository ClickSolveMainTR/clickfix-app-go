import { cn } from "@/lib/utils";

/** Colored dot of a custom conversation status. */
export function StatusDot({
  color,
  className,
}: {
  color?: string | null;
  className?: string;
}) {
  return (
    <span
      aria-hidden
      className={cn(
        "bg-muted-foreground inline-block size-2 shrink-0 rounded-full",
        className,
      )}
      style={color ? { backgroundColor: color } : undefined}
    />
  );
}
