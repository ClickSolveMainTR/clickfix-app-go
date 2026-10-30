"use client";

import {
  ArrowRight,
  Bot,
  Check,
  FileLock2,
  KeyRound,
  Layers,
  Webhook,
  type LucideIcon,
} from "lucide-react";
import Link from "next/link";

import { DOCS_PATH } from "@/features/landing/constants";
import {
  securitySections,
  type SecuritySection,
} from "@/features/landing/content/security";
import { landingHref, localizedHref } from "@/features/landing/lib/locale";

import { FinalCta } from "./final-cta";
import { useLanding } from "./landing-context";
import { GhostLink, PageHero } from "./page-hero";
import { Reveal, trackSpot } from "./section";

const ICONS: Record<SecuritySection["id"], LucideIcon> = {
  kvkk: FileLock2,
  access: KeyRound,
  isolation: Layers,
  ai: Bot,
  channels: Webhook,
};

/** `/security`: security controls and the KVKK tools, with doc links. */
export function SecurityPage({
  docs,
}: {
  docs: { slug: string; title: string; description: string }[];
}) {
  const { t, locale } = useLanding();
  const sections = securitySections(locale);

  return (
    <>
      <PageHero
        crumbs={[
          { href: landingHref(locale), label: t("landing.nav.home_short") },
          { label: t("landing.nav.security") },
        ]}
        eyebrow={t("landing.security.eyebrow")}
        title={t("landing.security.title")}
        lead={t("landing.security.lead")}
        actions={
          <>
            <GhostLink href={localizedHref("/legal/data-protection", locale)}>
              {t("landing.legal.kvkk_title")}
              <ArrowRight className="size-4" aria-hidden />
            </GhostLink>
          </>
        }
        aside={
          <ul
            aria-hidden
            className="grid grid-cols-2 gap-3 rounded-[2rem] border border-white/10 bg-[#0F1310]/80 p-4 backdrop-blur sm:grid-cols-3"
          >
            {["KVKK", "SAML", "OIDC", "SCIM", "2FA", "Passkey"].map((label) => (
              <li
                key={label}
                className="flex aspect-square flex-col items-center justify-center gap-2 rounded-2xl border border-white/8 bg-white/[0.03]"
              >
                <Check className="size-4 text-[#03FF13]" />
                <span className="font-display text-lg font-semibold tracking-tight">
                  {label}
                </span>
              </li>
            ))}
          </ul>
        }
      />

      <section className="py-20 sm:py-28">
        <div className="mx-auto grid max-w-7xl gap-4 px-4 sm:px-6 md:grid-cols-2">
          {sections.map((section, i) => {
            const Icon = ICONS[section.id];
            return (
              <Reveal
                key={section.id}
                delay={(i % 2) * 0.06}
                className={i === 0 ? "md:col-span-2" : undefined}
              >
                <article
                  id={section.id}
                  onPointerMove={trackSpot}
                  className="landing-spot flex h-full scroll-mt-24 flex-col gap-5 rounded-3xl border border-white/10 bg-[#0F1310] p-6 sm:p-8"
                >
                  <span className="flex size-10 items-center justify-center rounded-xl bg-white/[0.06] ring-1 ring-white/10">
                    <Icon className="size-5 text-[#7DFF86]" aria-hidden />
                  </span>
                  <div className="space-y-2">
                    <h2 className="font-display text-2xl font-semibold tracking-tight">
                      {section.title}
                    </h2>
                    <p className="max-w-2xl text-[15px] leading-relaxed text-white/60">
                      {section.body}
                    </p>
                  </div>
                  <ul
                    className={
                      i === 0
                        ? "grid gap-x-8 sm:grid-cols-2 lg:grid-cols-3"
                        : "grid gap-x-8"
                    }
                  >
                    {section.items.map((item) => (
                      <li
                        key={item}
                        className="flex items-start gap-3 border-t border-white/8 py-3 text-sm text-white/80"
                      >
                        <Check
                          className="mt-0.5 size-4 shrink-0 text-[#03FF13]"
                          aria-hidden
                        />
                        {item}
                      </li>
                    ))}
                  </ul>
                </article>
              </Reveal>
            );
          })}
        </div>

        {docs.length > 0 ? (
          <div className="mx-auto mt-16 max-w-7xl px-4 sm:px-6">
            <p className="font-accent mb-4 text-[11px] font-semibold tracking-[0.22em] text-white/45 uppercase">
              {t("landing.features.docs_title")}
            </p>
            <ul className="grid gap-3 md:grid-cols-2">
              {docs.map((doc) => (
                <li key={doc.slug}>
                  <Link
                    href={localizedHref(`${DOCS_PATH}/${doc.slug}`, locale)}
                    className="group flex h-full flex-col gap-1.5 rounded-2xl border border-white/10 bg-white/[0.02] p-5 transition-colors hover:border-white/25"
                  >
                    <span className="flex items-center justify-between gap-3 font-medium">
                      {doc.title}
                      <ArrowRight
                        aria-hidden
                        className="size-4 text-white/35 transition-transform group-hover:translate-x-0.5"
                      />
                    </span>
                    <span className="text-sm text-white/55">
                      {doc.description}
                    </span>
                  </Link>
                </li>
              ))}
            </ul>
          </div>
        ) : null}
      </section>

      <FinalCta />
    </>
  );
}
