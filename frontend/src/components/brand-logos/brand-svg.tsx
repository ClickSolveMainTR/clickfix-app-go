import type { ReactNode, SVGProps } from "react";

/**
 * Background the logo sits on. `auto` follows the panel theme; `light` and
 * `dark` pin a variant (the landing page is always dark).
 */
export type BrandTone = "auto" | "light" | "dark";

export type BrandLogoProps = Omit<SVGProps<SVGSVGElement>, "children"> & {
  /** Accessible name; defaults to the brand name. */
  title?: string;
  /** Hide from assistive tech when a visible label sits next to the logo. */
  decorative?: boolean;
  tone?: BrandTone;
};

/**
 * Fill classes for single-colour marks that use the brand's white variant on
 * dark backgrounds. `onLight` must be a literal class so Tailwind sees it.
 */
export function monoFill(tone: BrandTone = "auto", onLight: string) {
  if (tone === "dark") return "fill-white";
  if (tone === "light") return onLight;
  return `${onLight} dark:fill-white`;
}

/**
 * Shared `<svg>` shell: keeps the brand's aspect ratio (height follows the
 * class, width follows the viewBox) and gives it an accessible name.
 */
export function BrandSvg({
  name,
  viewBox,
  title,
  decorative,
  tone: _tone,
  children,
  ...props
}: BrandLogoProps & { name: string; viewBox: string; children: ReactNode }) {
  const label = title ?? name;
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      viewBox={viewBox}
      preserveAspectRatio="xMidYMid meet"
      focusable="false"
      {...(decorative
        ? { "aria-hidden": true }
        : { role: "img", "aria-label": label })}
      {...props}
    >
      {decorative ? null : <title>{label}</title>}
      {children}
    </svg>
  );
}
