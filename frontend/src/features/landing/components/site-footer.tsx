"use client";

import Link from "next/link";

import { AppWordmark } from "@/components/brand";
import { routes } from "@/config/routes";
import {
  COMPARE_PATH,
  CONTACT_PATH,
  DATA_DELETION_PATH,
  DOWNLOAD_PATH,
  DOCS_PATH,
  FEATURES_PATH,
  LEGAL_DOCS,
  LEGAL_KEYS,
  SECURITY_PATH,
} from "@/features/landing/constants";
import { featureCopy } from "@/features/landing/content/features";
import { FEATURE_AREAS, featureHref } from "@/features/landing/content/areas";
import { landingHref, localizedHref } from "@/features/landing/lib/locale";
import { CONSENT_OPEN_EVENT } from "@/features/landing/lib/tracking";

import { useLanding } from "./landing-context";

function FooterColumn({
  title,
  links,
}: {
  title: string;
  links: { href: string; label: string }[];
}) {
  return (
    <nav aria-label={title}>
      <p className="font-accent mb-4 text-[11px] tracking-[0.22em] text-white/60 uppercase">
        {title}
      </p>
      <ul className="space-y-2.5 text-sm">
        {links.map((link) => (
          <li key={link.href}>
            {link.href.startsWith("/platform/") ? (
              // App shell pages load fresh: marketing tags stay on this site.
              <a
                href={link.href}
                className="text-white/70 transition-colors hover:text-white"
              >
                {link.label}
              </a>
            ) : (
              <Link
                href={link.href}
                className="text-white/70 transition-colors hover:text-white"
              >
                {link.label}
              </Link>
            )}
          </li>
        ))}
      </ul>
    </nav>
  );
}

/** Footer of every marketing and docs page: product, resources, legal. */
export function SiteFooter() {
  const { t, locale } = useLanding();
  const copy = featureCopy(locale);

  const product = [
    {
      href: localizedHref(FEATURES_PATH, locale),
      label: t("landing.nav.features"),
    },
    ...FEATURE_AREAS.map((area) => ({
      href: featureHref(area, locale),
      label: copy[area.slug].name,
    })),
  ];
  const resources = [
    { href: localizedHref(DOCS_PATH, locale), label: t("landing.nav.docs") },
    {
      href: localizedHref(DOWNLOAD_PATH, locale),
      label: t("landing.nav.download"),
    },
    {
      href: localizedHref(COMPARE_PATH, locale),
      label: t("landing.nav.compare"),
    },
    {
      href: localizedHref(SECURITY_PATH, locale),
      label: t("landing.nav.security"),
    },
    {
      href: `${landingHref(locale)}#pricing`,
      label: t("landing.nav.pricing"),
    },
    { href: `${landingHref(locale)}#faq`, label: t("landing.nav.faq") },
    {
      href: localizedHref(CONTACT_PATH, locale),
      label: t("landing.nav.contact"),
    },
    { href: routes.guest.login, label: t("landing.nav.login") },
  ];
  const legal = [
    ...LEGAL_DOCS.map((doc) => ({
      href: localizedHref(`/legal/${doc}`, locale),
      label: t(`landing.legal.${LEGAL_KEYS[doc]}_title`),
    })),
    {
      href: localizedHref(DATA_DELETION_PATH, locale),
      label: t("landing.footer.data_deletion"),
    },
  ];

  return (
    <footer className="border-t border-white/8 pt-16 pb-10">
      <div className="mx-auto grid max-w-7xl gap-10 px-4 sm:grid-cols-2 sm:px-6 lg:grid-cols-[1.3fr_1fr_1fr_1fr]">
        <div className="space-y-4">
          <AppWordmark tone="dark" className="text-2xl" />
          <p className="max-w-sm text-sm leading-relaxed text-white/55">
            {t("landing.footer.tagline")}
          </p>
        </div>
        <FooterColumn title={t("landing.footer.product")} links={product} />
        <FooterColumn title={t("landing.footer.resources")} links={resources} />
        <FooterColumn title={t("landing.footer.legal")} links={legal} />
      </div>
      <div className="mx-auto mt-14 flex max-w-7xl flex-col gap-2 border-t border-white/8 px-4 pt-6 text-xs text-white/60 sm:flex-row sm:items-center sm:justify-between sm:px-6">
        <p>
          {t("landing.footer.copyright", { year: new Date().getFullYear() })}
        </p>
        <p>Powered by EasyTrade Yazılım Teknolojileri · Oğuzhan Yılmaz</p>
        <button
          type="button"
          onClick={() => window.dispatchEvent(new Event(CONSENT_OPEN_EVENT))}
          className="self-start underline-offset-2 hover:text-white hover:underline sm:self-auto"
        >
          {t("landing.footer.cookie_settings")}
        </button>
        <p>{t("landing.footer.made_in")}</p>
      </div>
    </footer>
  );
}
