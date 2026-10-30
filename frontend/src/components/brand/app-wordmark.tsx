import { brand, type BrandTone } from "@/config/brand";
import { cn } from "@/lib/utils";

import { AppMark } from "./app-mark";

type AppWordmarkProps = {
  title?: string;
  className?: string;
  tone?: BrandTone;
};

/**
 * Horizontal lockup: the mark and the name set in Space Grotesk. The kit has
 * no drawn wordmark, so the name is typeset (docs/brand.md). Size it with a
 * `text-*` class on `className`; the mark scales with the type.
 */
export function AppWordmark({
  className,
  title = brand.name,
  tone = "auto",
}: AppWordmarkProps) {
  return (
    <span
      className={cn(
        "font-display inline-flex items-center gap-[0.34em] text-[1.35rem] leading-none font-semibold tracking-[-0.04em]",
        tone === "dark" && "text-white",
        tone === "light" && "text-ink",
        className,
      )}
    >
      <AppMark tone={tone} title={title} className="size-[1.2em] shrink-0" />
      <span aria-hidden>{brand.wordmark}</span>
    </span>
  );
}
