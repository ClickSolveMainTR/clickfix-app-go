/**
 * Option colors are stored as a palette name; a `#rrggbb` value set through
 * the API is honoured too.
 */
export const OPTION_COLORS = [
  "gray",
  "red",
  "orange",
  "amber",
  "green",
  "teal",
  "blue",
  "violet",
  "pink",
] as const;

export type OptionColor = (typeof OPTION_COLORS)[number];

const BADGE_CLASSES: Record<OptionColor, string> = {
  gray: "bg-zinc-500/15 text-zinc-700 dark:text-zinc-300",
  red: "bg-red-500/15 text-red-700 dark:text-red-300",
  orange: "bg-orange-500/15 text-orange-700 dark:text-orange-300",
  amber: "bg-amber-500/15 text-amber-700 dark:text-amber-300",
  green: "bg-emerald-500/15 text-emerald-700 dark:text-emerald-300",
  teal: "bg-teal-500/15 text-teal-700 dark:text-teal-300",
  blue: "bg-blue-500/15 text-blue-700 dark:text-blue-300",
  violet: "bg-violet-500/15 text-violet-700 dark:text-violet-300",
  pink: "bg-pink-500/15 text-pink-700 dark:text-pink-300",
};

const SWATCH_CLASSES: Record<OptionColor, string> = {
  gray: "bg-zinc-500",
  red: "bg-red-500",
  orange: "bg-orange-500",
  amber: "bg-amber-500",
  green: "bg-emerald-500",
  teal: "bg-teal-500",
  blue: "bg-blue-500",
  violet: "bg-violet-500",
  pink: "bg-pink-500",
};

function isPalette(color: string): color is OptionColor {
  return (OPTION_COLORS as readonly string[]).includes(color);
}

const HEX_RE = /^#[0-9a-f]{6}$/i;

/** Class names and inline style for an option badge. */
export function optionBadgeStyle(color?: string | null): {
  className: string;
  style?: { backgroundColor: string; color: string };
} {
  if (color && isPalette(color)) {
    return { className: `border-transparent ${BADGE_CLASSES[color]}` };
  }
  if (color && HEX_RE.test(color)) {
    return {
      className: "border-transparent",
      style: { backgroundColor: `${color}26`, color },
    };
  }
  return {
    className: "border-transparent bg-secondary text-secondary-foreground",
  };
}

export function optionSwatchClass(color?: string | null): string {
  return color && isPalette(color)
    ? SWATCH_CLASSES[color]
    : "bg-muted-foreground/40";
}
