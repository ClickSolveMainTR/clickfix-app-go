import type { CSSProperties } from "react";
import type { SimpleIcon } from "simple-icons";

import { cn } from "@/lib/utils";

import { BrandSvg, type BrandLogoProps, type BrandTone } from "./brand-svg";

/** WCAG relative luminance of a `RRGGBB` colour. */
function luminance(hex: string) {
  const [r, g, b] = [0, 2, 4].map((i) => {
    const c = parseInt(hex.slice(i, i + 2), 16) / 255;
    return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
  });
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}

/** Contrast of a colour against the dark panel background (~#0a0a0a). */
function contrastOnDark(hex: string) {
  return (luminance(hex) + 0.05) / (0.003 + 0.05);
}

/** Contrast of a colour against a white background. */
function contrastOnLight(hex: string) {
  return 1.05 / (luminance(hex) + 0.05);
}

/**
 * Very light brand colours (React cyan, Uptime Kuma green...) keep their
 * colour on light backgrounds and get a hairline outline instead.
 */
function outlineClass(tone: BrandTone = "auto") {
  if (tone === "dark") return undefined;
  if (tone === "light") return "drop-shadow-[0_0_0.5px_rgb(0_0_0/0.55)]";
  return "drop-shadow-[0_0_0.5px_rgb(0_0_0/0.55)] dark:drop-shadow-none";
}

/**
 * Fill class for a Simple Icons glyph. Marks in the brand colour stay in that
 * colour; colours that vanish on the dark theme (black, navy...) switch to
 * white there, which is the white variant those brands publish.
 */
function fillClass(tone: BrandTone = "auto", invertsOnDark: boolean) {
  if (!invertsOnDark) return "fill-(--brand-fill)";
  if (tone === "dark") return "fill-white";
  if (tone === "light") return "fill-(--brand-fill)";
  return "fill-(--brand-fill) dark:fill-white";
}

/**
 * Logo component for a single-colour mark from the `simple-icons` package
 * (CC0 path data, official brand hex). `name` overrides the package title.
 */
export function simpleIconLogo(icon: SimpleIcon, name = icon.title) {
  const invertsOnDark = contrastOnDark(icon.hex) < 3;
  const fadesOnLight = contrastOnLight(icon.hex) < 2;
  function SimpleIconLogo({
    tone,
    style,
    className,
    ...props
  }: BrandLogoProps) {
    return (
      <BrandSvg
        name={name}
        viewBox="0 0 24 24"
        style={{ "--brand-fill": `#${icon.hex}`, ...style } as CSSProperties}
        className={cn(fadesOnLight && outlineClass(tone), className)}
        {...props}
      >
        <path className={fillClass(tone, invertsOnDark)} d={icon.path} />
      </BrandSvg>
    );
  }
  SimpleIconLogo.displayName = `${icon.slug}Logo`;
  return SimpleIconLogo;
}
