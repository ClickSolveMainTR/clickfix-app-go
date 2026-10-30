import { cn } from "@/lib/utils";

/** Circular progress in the signal colour, with the count in the middle. */
export function ProgressRing({
  value,
  total,
  className,
}: {
  value: number;
  total: number;
  className?: string;
}) {
  const r = 22;
  const c = 2 * Math.PI * r;
  const ratio = total > 0 ? Math.min(1, value / total) : 0;
  return (
    <span
      className={cn("relative inline-flex size-14 shrink-0", className)}
      role="img"
      aria-label={`${value}/${total}`}
    >
      <svg viewBox="0 0 56 56" className="size-full -rotate-90">
        <circle
          cx="28"
          cy="28"
          r={r}
          fill="none"
          strokeWidth="5"
          className="stroke-muted"
        />
        <circle
          cx="28"
          cy="28"
          r={r}
          fill="none"
          strokeWidth="5"
          strokeLinecap="round"
          strokeDasharray={c}
          strokeDashoffset={c * (1 - ratio)}
          className="stroke-signal transition-[stroke-dashoffset] duration-700 ease-out motion-reduce:transition-none"
        />
      </svg>
      <span className="font-accent absolute inset-0 flex items-center justify-center text-sm font-semibold tabular-nums">
        {value}/{total}
      </span>
    </span>
  );
}
