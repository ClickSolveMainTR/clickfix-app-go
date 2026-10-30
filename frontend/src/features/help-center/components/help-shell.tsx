import { Search } from "lucide-react";
import Link from "next/link";
import type { CSSProperties, ReactNode } from "react";

import { AppWordmark } from "@/components/brand/app-wordmark";
import { brand } from "@/config/brand";
import { routes } from "@/config/routes";
import { translate } from "@/lib/i18n/messages";

import {
  helpLogoSrc,
  type HelpLocale,
  type HelpOrganization,
} from "../lib/api";

/** Text colour (ink or white) with the better contrast on a hex background. */
export function contrastText(hex: string): string {
  const n = Number.parseInt(hex.slice(1), 16);
  const channel = (v: number) => {
    const c = v / 255;
    return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
  };
  const lum =
    0.2126 * channel((n >> 16) & 255) +
    0.7152 * channel((n >> 8) & 255) +
    0.0722 * channel(n & 255);
  // Contrast against white vs. against black; pick the larger.
  return 1.05 / (lum + 0.05) >= (lum + 0.05) / 0.05 ? "#ffffff" : "#000000";
}

type HelpShellProps = {
  org: HelpOrganization;
  locale: HelpLocale;
  /** Current path without the language query (language switch links). */
  path: string;
  query?: string;
  /** Bigger header with the search box as the hero (home page). */
  hero?: boolean;
  children: ReactNode;
};

/**
 * Public help center frame (CF-74): organization branding (logo, name,
 * accent colour of its web widget), search, language switch and footer.
 * Server component; no session, no client JS.
 */
export function HelpShell({
  org,
  locale,
  path,
  query,
  hero = false,
  children,
}: HelpShellProps) {
  const tc = (key: string, params?: Record<string, string | number>) =>
    translate(locale, `help_center.${key}`, params);
  const accent = org.accent_color ?? brand.colors.ink;
  const headerStyle: CSSProperties = {
    backgroundColor: accent,
    color: contrastText(accent),
  };
  const logo = helpLogoSrc(org);
  const locales = org.locales.map((l) => l.locale);
  const other = locale === "tr" ? "en" : "tr";

  return (
    <div className="bg-background flex min-h-screen flex-col" lang={locale}>
      <a
        href="#help-main"
        className="focus:bg-background sr-only focus:not-sr-only focus:absolute focus:top-2 focus:left-2 focus:z-50 focus:rounded-md focus:px-3 focus:py-2"
      >
        {tc("skip_to_content")}
      </a>
      <header style={headerStyle}>
        <div className="mx-auto flex max-w-5xl items-center justify-between gap-4 px-4 py-4 sm:px-6">
          <Link
            href={routes.help.home(org.slug, locale)}
            className="flex min-w-0 items-center gap-3 rounded-md outline-none focus-visible:ring-2 focus-visible:ring-current"
          >
            {logo ? (
              // eslint-disable-next-line @next/next/no-img-element
              <img
                src={logo}
                alt=""
                className="size-9 shrink-0 rounded-lg bg-white object-contain p-0.5"
              />
            ) : null}
            <span className="font-display truncate text-lg font-semibold tracking-tight">
              {org.name}
            </span>
            <span className="hidden text-sm opacity-75 sm:inline">
              {tc("title")}
            </span>
          </Link>
          {locales.includes(other) && (
            <Link
              href={
                other === "tr"
                  ? path
                  : `${path}${path.includes("?") ? "&" : "?"}lang=en`
              }
              hrefLang={other}
              lang={other}
              className="shrink-0 rounded-md px-2 py-1 text-sm font-medium underline-offset-4 opacity-90 outline-none hover:underline focus-visible:ring-2 focus-visible:ring-current"
            >
              {tc(`switch_to_${other}`)}
            </Link>
          )}
        </div>
        <div
          className={
            hero
              ? "mx-auto max-w-5xl px-4 pt-6 pb-14 sm:px-6 sm:pt-10 sm:pb-20"
              : "mx-auto max-w-5xl px-4 pb-6 sm:px-6"
          }
        >
          {hero && (
            <h1 className="font-display mb-5 text-3xl font-semibold tracking-tight text-balance sm:text-4xl">
              {tc("hero_title")}
            </h1>
          )}
          <form
            action={`/help/${encodeURIComponent(org.slug)}/search`}
            method="get"
            role="search"
            className="relative max-w-2xl"
          >
            <label htmlFor="help-search" className="sr-only">
              {tc("search_label")}
            </label>
            <Search
              aria-hidden
              className="text-muted-foreground pointer-events-none absolute top-1/2 left-4 size-5 -translate-y-1/2"
            />
            <input
              id="help-search"
              name="q"
              type="search"
              defaultValue={query}
              maxLength={200}
              required
              placeholder={tc("search_placeholder")}
              className="bg-background text-foreground placeholder:text-muted-foreground h-12 w-full rounded-xl border-0 pr-4 pl-12 text-base shadow-lg ring-1 ring-black/10 outline-none focus-visible:ring-2 focus-visible:ring-black/40"
            />
            {locale !== "tr" && (
              <input type="hidden" name="lang" value={locale} />
            )}
          </form>
        </div>
      </header>
      <main
        id="help-main"
        className="mx-auto w-full max-w-5xl flex-1 px-4 py-8 sm:px-6 sm:py-10"
      >
        {children}
      </main>
      <footer className="border-border border-t">
        <div className="text-muted-foreground mx-auto flex max-w-5xl flex-wrap items-center justify-between gap-3 px-4 py-6 text-sm sm:px-6">
          <span>
            © {new Date().getFullYear()} {org.name}
          </span>
          <a
            href={brand.siteUrl}
            className="flex items-center gap-2 rounded-md outline-none hover:opacity-80 focus-visible:ring-2"
            rel="noopener"
          >
            <span>{tc("powered_by")}</span>
            <AppWordmark className="text-sm" />
          </a>
        </div>
      </footer>
    </div>
  );
}
