/**
 * ClickFix identity (docs/brand.md). Hex aliases for JS (theme-color, icons,
 * shaders); CSS tokens live in styles/theme.css.
 */
export const brand = {
  name: "ClickFix",
  productName: "ClickFix",
  /** Typeset wordmark (the kit has no drawn letters). */
  wordmark: "clickfix",
  tagline: "Müşteri destek platformu",
  /** Production origin (marketing + app). Override with SITE_URL / AUTH_URL. */
  siteUrl: "https://getclickfix.com",
  /** Public mailboxes (docs/DEVELOPMENT.md "Yayınlama"). */
  supportEmail: "destek@getclickfix.com",
  /** Custom URL scheme of the companion mobile app (QR sign-in fallback). */
  mobileScheme: "clickfix",
  colors: {
    /** Ink: the kit black (mark disc, tile, text on signal, sidebar card). */
    ink: "#000000",
    /** Dark page surface: lifted from ink so the black mark stays visible. */
    surface: "#0A0D0B",
    /** Raised ink (dark cards). */
    inkRaised: "#111412",
    /** Warm paper: light-mode page background. */
    paper: "#F7F6F2",
    /** Electric green: the mark's cursor, the signal colour. Fill only. */
    signal: "#03FF13",
    /** Green for text / links on light surfaces (5.6:1 on paper). */
    signalText: "#087313",
    white: "#FFFFFF",
    /** Light-mode primary (ink) and its text. */
    primary: "#0C110D",
    primaryForeground: "#F7F6F2",
    /** Compat aliases used by theme-switch / settings defaults. */
    navy: "#000000",
    lightGray: "#EFEEE9",
  },
  /** Shader palettes (auth backdrop, landing hero): ink → deep green → signal. */
  shader: {
    mesh: ["#0A0D0B", "#0E2412", "#0A0D0B", "#164D1E", "#0A0D0B", "#03FF13"],
    grain: ["#0A0D0B", "#123A18", "#03FF13"],
  },
} as const;

export type BrandTone = "auto" | "light" | "dark";
export type BrandVariant = "logo" | "wordmark" | "mark" | "icon";
