import type { SVGProps } from "react";

import { brand, type BrandTone } from "@/config/brand";
import { cn } from "@/lib/utils";

import {
  CURSOR_ONLY_POINTS,
  CURSOR_ONLY_VIEWBOX,
  MARK_CURSOR_POINTS,
  MARK_DISC,
  MARK_VIEWBOX,
} from "./clickfix-paths";

type AppMarkProps = Omit<SVGProps<SVGSVGElement>, "children"> & {
  title?: string;
  /**
   * Background the mark sits on. `auto` adds a hairline ring around the ink
   * disc in dark mode so it does not melt into ink surfaces; `dark` always
   * draws the ring; `light` never does.
   */
  tone?: BrandTone;
  /** `cursor` draws only the arrow in `currentColor` (tiny glyph uses). */
  glyph?: "mark" | "cursor";
};

/** ClickFix mark: official circle, ink disc + signal-green cursor (docs/brand.md). */
export function AppMark({
  className,
  title = brand.name,
  tone = "auto",
  glyph = "mark",
  ...props
}: AppMarkProps) {
  if (glyph === "cursor") {
    return (
      <svg
        viewBox={CURSOR_ONLY_VIEWBOX}
        xmlns="http://www.w3.org/2000/svg"
        role="img"
        aria-label={title}
        className={cn("size-4", className)}
        {...props}
      >
        <title>{title}</title>
        <polygon fill="currentColor" points={CURSOR_ONLY_POINTS} />
      </svg>
    );
  }

  return (
    <svg
      viewBox={MARK_VIEWBOX}
      xmlns="http://www.w3.org/2000/svg"
      role="img"
      aria-label={title}
      className={cn("size-4", className)}
      {...props}
    >
      <title>{title}</title>
      <circle
        cx={MARK_DISC.cx}
        cy={MARK_DISC.cy}
        r={MARK_DISC.r - 24}
        fill={brand.colors.ink}
        strokeWidth={48}
        className={cn(
          tone === "dark" && "stroke-white/20",
          tone === "auto" && "stroke-transparent dark:stroke-white/20",
          tone === "light" && "stroke-transparent",
        )}
      />
      <polygon fill={brand.colors.signal} points={MARK_CURSOR_POINTS} />
    </svg>
  );
}
