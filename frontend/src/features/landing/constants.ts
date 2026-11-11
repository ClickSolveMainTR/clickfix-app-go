/**
 * Top-level marketing pages (nav and footer). `hash` entries are sections of
 * the home page; the others are their own routes.
 */
export const SITE_PAGES = [
  { key: "features", path: "/features" },
  { key: "pricing", path: "/", hash: "pricing" },
  { key: "compare", path: "/compare" },
  { key: "security", path: "/security" },
  { key: "docs", path: "/docs" },
] as const;

/** Canonical registration page, controlled by the platform registration setting. */
export const SIGNUP_PATH = "/register";

export const COMPARE_PATH = "/compare";
export const SECURITY_PATH = "/security";
export const CONTACT_PATH = "/contact";
export const FEATURES_PATH = "/features";
export const DOCS_PATH = "/docs";

/** Home FAQ entries (`landing.faq.<key>_q` / `_a`), also the FAQPage JSON-LD. */
export const FAQ_KEYS = [
  "channels",
  "ai",
  "kvkk",
  "trial",
  "payment",
  "mobile",
  "migration",
  "video",
  "api",
  "sso",
  "languages",
  "sdk",
  "assistant",
] as const;

/** Legal documents linked from the footer (`/legal/<slug>`). */
export const LEGAL_DOCS = [
  "data-protection",
  "privacy",
  "terms",
  "cookies",
] as const;
export type LegalDoc = (typeof LEGAL_DOCS)[number];

/**
 * Public data deletion instructions (CF-135; Meta app review). Linked from
 * the footer next to the legal documents. `items` is the number of
 * `landing.deletion.<id>.item_<n>` keys of each section.
 */
export const DATA_DELETION_PATH = "/account-deletion";
export const DATA_DELETION_SECTIONS = [
  { id: "account", items: 4 },
  { id: "organization", items: 5 },
  { id: "customers", items: 2 },
  { id: "meta", items: 5 },
  { id: "timeline", items: 4 },
] as const;

/**
 * Sales / demo contact ("Satışla görüşün", demo CTA). The pricing section is a
 * client component, so the env value is inlined at build time; the Docker
 * build does not receive it, hence the product inbox as default (CF-15:
 * destek@getclickfix.com). Without any address the CTAs fell back to sign-up.
 */
export const contactEmail =
  process.env.NEXT_PUBLIC_CONTACT_EMAIL || "destek@getclickfix.com";

export const LEGAL_KEYS = {
  "data-protection": "kvkk",
  privacy: "gizlilik",
  terms: "kosullar",
  cookies: "cerezler",
} as const;
