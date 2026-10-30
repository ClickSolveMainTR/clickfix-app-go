"use client";

import { ArrowRight } from "lucide-react";

import { COMPARE_PATH } from "@/features/landing/constants";
import { landingHref, localizedHref } from "@/features/landing/lib/locale";

import { FinalCta } from "./final-cta";
import { LandingCta } from "./landing-cta";
import { useLanding } from "./landing-context";
import { ModulesGrid } from "./modules-grid";
import { GhostLink, PageHero } from "./page-hero";

/** `/features`: the product areas as a grid of linked cards. */
export function FeaturesIndex() {
  const { t, locale } = useLanding();

  return (
    <>
      <PageHero
        compact
        crumbs={[
          { href: landingHref(locale), label: t("landing.nav.home_short") },
          { label: t("landing.nav.features") },
        ]}
        eyebrow={t("landing.features.eyebrow")}
        title={t("landing.features.title")}
        lead={t("landing.features.lead")}
        actions={
          <>
            <LandingCta />
            <GhostLink href={localizedHref(COMPARE_PATH, locale)}>
              {t("landing.features.compare_link")}
              <ArrowRight className="size-4" aria-hidden />
            </GhostLink>
          </>
        }
      />
      <section className="pb-24 sm:pb-32">
        <div className="mx-auto max-w-7xl px-4 sm:px-6">
          <ModulesGrid />
        </div>
      </section>
      <FinalCta />
    </>
  );
}
