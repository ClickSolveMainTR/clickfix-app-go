import type { LucideIcon } from "lucide-react";

import type { BrandLogoProps } from "./brand-svg";
import { BRAND_LOGOS, type BrandSlug } from "./registry";

/** Original logo of an external company, e.g. `<BrandLogo slug="whatsapp" />`. */
export function BrandLogo({
  slug,
  ...props
}: BrandLogoProps & { slug: BrandSlug }) {
  const Logo = BRAND_LOGOS[slug];
  return <Logo {...props} />;
}

const navIcons = new Map<BrandSlug, LucideIcon>();

/**
 * Lucide-compatible wrapper for places typed as `LucideIcon` (sidebar,
 * settings tabs, command palette). Decorative: those places print the label.
 */
export function brandNavIcon(slug: BrandSlug): LucideIcon {
  let icon = navIcons.get(slug);
  if (!icon) {
    const Logo = BRAND_LOGOS[slug];
    const NavIcon = ({ className }: { className?: string }) => (
      <Logo className={className} decorative />
    );
    NavIcon.displayName = `${slug}NavIcon`;
    icon = NavIcon as LucideIcon;
    navIcons.set(slug, icon);
  }
  return icon;
}
