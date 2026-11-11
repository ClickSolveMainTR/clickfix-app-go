"use client";

import { Moon, Sun } from "lucide-react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { useTheme } from "next-themes";
import { useSyncExternalStore } from "react";

import { i18nConfig, type AppLocale } from "@/config/i18n";
import { localizedHref } from "@/features/landing/lib/locale";
import { cn } from "@/lib/utils";

const LOCALES: { value: AppLocale; label: string }[] = [
  { value: "tr", label: "Türkçe" },
  { value: "en", label: "English" },
];

const noop = () => () => {};

/**
 * Docs sidebar banner: the same page in the other language and the colour
 * mode. Language is the marketing site's `?lang=en` convention (the proxy
 * rewrites it to `/en/docs/...`); the theme is the app-wide next-themes
 * class, so a reader who picks light docs gets a light panel too.
 */
export function DocsToolbar({ locale }: { locale: AppLocale }) {
  const pathname = usePathname();
  const { resolvedTheme, setTheme } = useTheme();
  // Theme is unknown until hydration; render the toggle neutral on the server.
  const mounted = useSyncExternalStore(
    noop,
    () => true,
    () => false,
  );
  const dark = mounted && resolvedTheme === "dark";
  const path = pathname.replace(/^\/en(?=\/|$)/, "") || "/";
  const label = (tr: string, en: string) => (locale === "tr" ? tr : en);

  return (
    <div className="border-fd-border flex items-center justify-between gap-2 border-b pb-3">
      <nav
        aria-label={label("Dil", "Language")}
        className="bg-fd-muted flex items-center gap-1 rounded-full p-0.5"
      >
        {LOCALES.map((option) => (
          <Link
            key={option.value}
            href={localizedHref(path, option.value)}
            hrefLang={option.value}
            lang={option.value}
            aria-current={option.value === locale ? "true" : undefined}
            onClick={() => {
              try {
                window.localStorage.setItem(
                  i18nConfig.storageKey,
                  option.value,
                );
              } catch {
                // Private mode / blocked storage: the link still works.
              }
            }}
            className={cn(
              "rounded-full px-2.5 py-1 text-xs font-medium transition-colors",
              option.value === locale
                ? "bg-fd-background text-fd-foreground shadow-sm"
                : "text-fd-muted-foreground hover:text-fd-foreground",
            )}
          >
            {option.label}
          </Link>
        ))}
      </nav>
      <button
        type="button"
        onClick={() => setTheme(dark ? "light" : "dark")}
        aria-label={label(
          dark ? "Açık temaya geç" : "Koyu temaya geç",
          dark ? "Switch to light mode" : "Switch to dark mode",
        )}
        title={label("Tema", "Theme")}
        className="text-fd-muted-foreground hover:bg-fd-accent hover:text-fd-accent-foreground inline-flex size-8 items-center justify-center rounded-full transition-colors"
      >
        {dark ? (
          <Sun aria-hidden className="size-4" />
        ) : (
          <Moon aria-hidden className="size-4" />
        )}
      </button>
    </div>
  );
}
