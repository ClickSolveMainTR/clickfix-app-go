import { useId, type ReactNode } from "react";

import { AppMark } from "@/components/brand";
import { cn } from "@/lib/utils";

type EmptyStateProps = {
  title: string;
  description?: string;
  action?: ReactNode;
  /** Replaces the cursor in the middle of the illustration. */
  icon?: ReactNode;
  className?: string;
};

/**
 * Brand illustration for empty screens: a dotted signal grid with radar
 * rings around a small ink tile. Pure SVG + CSS, no animation, so it costs
 * nothing and stays still for reduced motion.
 */
function SignalIllustration({ icon }: { icon?: ReactNode }) {
  const id = useId().replace(/:/g, "");
  return (
    <div aria-hidden className="relative size-28">
      <svg viewBox="0 0 112 112" className="absolute inset-0 size-full">
        <defs>
          <pattern
            id={`empty-dots-${id}`}
            width="8"
            height="8"
            patternUnits="userSpaceOnUse"
          >
            <circle cx="1" cy="1" r="0.9" className="fill-foreground/15" />
          </pattern>
          <radialGradient id={`empty-fade-${id}`}>
            <stop offset="35%" stopColor="#fff" stopOpacity="1" />
            <stop offset="100%" stopColor="#fff" stopOpacity="0" />
          </radialGradient>
          <mask id={`empty-mask-${id}`}>
            <rect width="112" height="112" fill={`url(#empty-fade-${id})`} />
          </mask>
        </defs>
        <rect
          width="112"
          height="112"
          fill={`url(#empty-dots-${id})`}
          mask={`url(#empty-mask-${id})`}
        />
        <circle
          cx="56"
          cy="56"
          r="40"
          fill="none"
          strokeDasharray="2 5"
          className="stroke-foreground/20"
        />
        <circle
          cx="56"
          cy="56"
          r="27"
          fill="none"
          className="stroke-foreground/10"
        />
        <circle cx="90" cy="35" r="3" className="fill-signal" />
        <circle
          cx="90"
          cy="35"
          r="6"
          fill="none"
          className="stroke-signal/40"
        />
      </svg>
      <span className="bg-ink absolute top-1/2 left-1/2 flex size-11 -translate-x-1/2 -translate-y-1/2 items-center justify-center rounded-xl shadow-lg ring-1 shadow-black/20 ring-white/10">
        {icon ?? (
          <AppMark glyph="cursor" className="text-signal size-5" title="" />
        )}
      </span>
    </div>
  );
}

export function EmptyState({
  title,
  description,
  action,
  icon,
  className,
}: EmptyStateProps) {
  return (
    <div
      className={cn(
        "border-border bg-card/40 flex flex-col items-center justify-center gap-3 rounded-xl border border-dashed px-6 py-14 text-center",
        className,
      )}
    >
      <SignalIllustration icon={icon} />
      <h3 className="font-display text-base font-semibold tracking-tight">
        {title}
      </h3>
      {description ? (
        <p className="text-muted-foreground max-w-md text-sm text-pretty">
          {description}
        </p>
      ) : null}
      {action ? <div className="mt-1">{action}</div> : null}
    </div>
  );
}
