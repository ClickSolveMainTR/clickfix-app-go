import { brand, type BrandTone, type BrandVariant } from "@/config/brand";
import { cn } from "@/lib/utils";

import { AppMark } from "./app-mark";
import { AppWordmark } from "./app-wordmark";
import { TILE_CURSOR_POINTS, TILE_VIEWBOX } from "./clickfix-paths";

type AppLogoProps = {
  variant?: BrandVariant;
  tone?: BrandTone;
  className?: string;
};

/**
 * ClickFix logo. `logo` / `wordmark` → mark + typeset name; `mark` → disc and
 * cursor; `icon` → the official green app-icon tile (black cursor on signal
 * green), the same art as the favicon and the Apple touch icon.
 */
export function AppLogo({
  variant = "logo",
  tone = "auto",
  className,
}: AppLogoProps) {
  if (variant === "mark") {
    return <AppMark tone={tone} className={cn("size-4 shrink-0", className)} />;
  }

  if (variant === "icon") {
    return (
      <svg
        viewBox={TILE_VIEWBOX}
        xmlns="http://www.w3.org/2000/svg"
        role="img"
        aria-label={brand.name}
        className={cn("size-8 shrink-0 rounded-[22%]", className)}
      >
        <title>{brand.name}</title>
        <rect width="2050" height="2050" fill={brand.colors.signal} />
        <polygon fill={brand.colors.ink} points={TILE_CURSOR_POINTS} />
      </svg>
    );
  }

  return <AppWordmark tone={tone} className={className} />;
}
