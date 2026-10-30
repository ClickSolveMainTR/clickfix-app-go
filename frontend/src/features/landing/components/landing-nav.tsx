"use client";

import { Menu } from "lucide-react";
import {
  motion,
  useMotionValueEvent,
  useScroll,
  useSpring,
} from "motion/react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { useEffect, useState } from "react";

import { AppWordmark } from "@/components/brand";
import {
  Sheet,
  SheetClose,
  SheetContent,
  SheetTitle,
  SheetTrigger,
} from "@/components/ui/sheet";
import { i18nConfig, type AppLocale } from "@/config/i18n";
import { useAuth } from "@/providers/auth-provider";
import { defaultHomeForUser } from "@/lib/auth/types";

import { routes } from "@/config/routes";
import { SITE_PAGES } from "@/features/landing/constants";
import {
  landingHref,
  localizedHref,
  sitePageHref,
} from "@/features/landing/lib/locale";
import { cn } from "@/lib/utils";

import { LandingCta } from "./landing-cta";
import { useLanding } from "./landing-context";

const LOCALES: { value: AppLocale; label: string }[] = [
  { value: "tr", label: "TR" },
  { value: "en", label: "EN" },
];

/**
 * Carry the marketing-site language into the app (register, set-up, inbox),
 * which reads its locale from localStorage: an English visitor used to land
 * in a Turkish product after sign-up.
 */
function rememberLocale(locale: AppLocale) {
  try {
    window.localStorage.setItem(i18nConfig.storageKey, locale);
  } catch {
    // Private mode / blocked storage: the app keeps its default locale.
  }
}

/**
 * Fixed top bar: transparent over the hero, ink glass once the page
 * scrolls, with a signal-green progress line along its bottom edge.
 */
export function LandingNav() {
  const { t, locale } = useLanding();
  const { isAuthenticated, user } = useAuth();
  const panelHref = user ? defaultHomeForUser(user) : routes.cms.home;
  const { scrollY, scrollYProgress } = useScroll();
  const progress = useSpring(scrollYProgress, { stiffness: 200, damping: 40 });
  const [scrolled, setScrolled] = useState(false);
  const pathname = usePathname();
  const isCurrent = (page: (typeof SITE_PAGES)[number]) =>
    !("hash" in page) && pathname.startsWith(page.path);

  useMotionValueEvent(scrollY, "change", (y) => setScrolled(y > 24));

  useEffect(() => {
    const aliases: Record<string, string> = {
      fiyatlar: "pricing",
      icerik: "content",
      urun: "product",
      kanallar: "channels",
      sss: "faq",
      nasil: "how-it-works",
    };
    const resolveHash = () => {
      const hash = aliases[window.location.hash.slice(1)];
      if (!hash) return;
      window.history.replaceState(
        null,
        "",
        `${window.location.pathname}${window.location.search}#${hash}`,
      );
      document.getElementById(hash)?.scrollIntoView();
    };
    resolveHash();
    window.addEventListener("hashchange", resolveHash);
    return () => window.removeEventListener("hashchange", resolveHash);
  }, []);

  // Only a non-default language is an explicit choice (`?lang=en`); Turkish
  // is stored when picked from the switcher, so a signed-in English user
  // opening / keeps English.
  useEffect(() => {
    if (locale !== i18nConfig.defaultLocale) rememberLocale(locale);
  }, [locale]);

  const languages = (
    <nav aria-label={t("landing.nav.language")} className="flex items-center">
      {LOCALES.map((option) => (
        <Link
          key={option.value}
          href={localizedHref(
            pathname.replace(/^\/en(?=\/|$)/, "") || "/",
            option.value,
          )}
          onClick={() => rememberLocale(option.value)}
          hrefLang={option.value}
          lang={option.value}
          aria-current={option.value === locale ? "true" : undefined}
          className={cn(
            "rounded-full px-2.5 py-1 font-mono text-[11px] tracking-widest transition-colors",
            option.value === locale
              ? "bg-white/12 text-white"
              : "text-white/55 hover:text-white",
          )}
        >
          {option.label}
        </Link>
      ))}
    </nav>
  );

  return (
    <header
      className={cn(
        "fixed inset-x-0 top-0 z-50 transition-[background-color,border-color,backdrop-filter] duration-500",
        scrolled
          ? "border-b border-white/8 bg-[#0A0D0B]/75 backdrop-blur-xl"
          : "border-b border-transparent",
      )}
    >
      <div className="mx-auto flex h-16 w-full max-w-7xl items-center justify-between gap-4 px-4 sm:px-6">
        <Link
          href={landingHref(locale)}
          aria-label={t("landing.nav.home")}
          className="shrink-0 rounded-md"
        >
          <AppWordmark tone="dark" className="text-[1.4rem]" />
        </Link>

        <nav
          aria-label={t("landing.nav.label")}
          className="hidden items-center gap-1 lg:flex"
        >
          {SITE_PAGES.map((page) => (
            <Link
              key={page.key}
              href={sitePageHref(page, locale)}
              aria-current={isCurrent(page) ? "page" : undefined}
              className={cn(
                "rounded-full px-3 py-1.5 text-sm transition-colors hover:bg-white/8 hover:text-white",
                isCurrent(page) ? "bg-white/8 text-white" : "text-white/70",
              )}
            >
              {t(`landing.nav.${page.key}`)}
            </Link>
          ))}
        </nav>

        <div className="flex items-center gap-2">
          <div className="hidden sm:block">{languages}</div>
          <a
            href={isAuthenticated ? panelHref : routes.guest.login}
            className="hidden rounded-full px-3 py-1.5 text-sm text-white/80 transition-colors hover:text-white sm:inline-flex"
          >
            {isAuthenticated
              ? locale === "tr"
                ? "Panele git"
                : "Go to dashboard"
              : t("landing.nav.login")}
          </a>
          {!isAuthenticated && (
            <LandingCta size="sm" className="hidden sm:inline-flex" />
          )}
          <Sheet>
            <SheetTrigger asChild>
              <button
                type="button"
                className="inline-flex size-10 items-center justify-center rounded-full text-white hover:bg-white/10 lg:hidden"
                aria-label={t("landing.nav.menu")}
              >
                <Menu aria-hidden className="size-5" />
              </button>
            </SheetTrigger>
            <SheetContent
              side="right"
              className="landing-root w-[min(22rem,100vw)] border-white/10 bg-[#0A0D0B] text-white"
            >
              <SheetTitle className="sr-only">
                {t("landing.nav.label")}
              </SheetTitle>
              <div className="flex h-full flex-col gap-8 p-6 pt-14">
                <nav className="flex flex-col gap-1">
                  {SITE_PAGES.map((page) => (
                    <SheetClose asChild key={page.key}>
                      <Link
                        href={sitePageHref(page, locale)}
                        aria-current={isCurrent(page) ? "page" : undefined}
                        className="font-display rounded-xl px-3 py-3 text-2xl font-semibold tracking-tight text-white/85 hover:bg-white/5 hover:text-white"
                      >
                        {t(`landing.nav.${page.key}`)}
                      </Link>
                    </SheetClose>
                  ))}
                </nav>
                <div className="mt-auto flex flex-col gap-4">
                  {languages}
                  <a
                    href={isAuthenticated ? panelHref : routes.guest.login}
                    className="text-sm text-white/70 hover:text-white"
                  >
                    {isAuthenticated
                      ? locale === "tr"
                        ? "Panele git"
                        : "Go to dashboard"
                      : t("landing.nav.login")}
                  </a>
                  {!isAuthenticated && <LandingCta className="w-full" />}
                </div>
              </div>
            </SheetContent>
          </Sheet>
        </div>
      </div>
      <motion.div
        aria-hidden
        style={{ scaleX: progress }}
        className="absolute inset-x-0 bottom-0 h-px origin-left bg-linear-to-r from-transparent via-[#03FF13] to-[#7DFF86]"
      />
    </header>
  );
}
