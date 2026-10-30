"use client";

import {
  ArrowUpRight,
  BookOpenText,
  CalendarClock,
  LifeBuoy,
  type LucideIcon,
} from "lucide-react";
import Link from "next/link";

import { brand } from "@/config/brand";
import { contactEmail, DOCS_PATH } from "@/features/landing/constants";
import { landingHref, localizedHref } from "@/features/landing/lib/locale";

import { LandingCta } from "./landing-cta";
import { useLanding } from "./landing-context";
import { PageHero } from "./page-hero";
import { Reveal, trackSpot } from "./section";

function ContactCard({
  icon: Icon,
  title,
  body,
  href,
  action,
  external,
}: {
  icon: LucideIcon;
  title: string;
  body: string;
  href: string;
  action: string;
  external?: boolean;
}) {
  const className =
    "landing-spot group flex h-full flex-col gap-5 rounded-3xl border border-white/10 bg-[#0F1310] p-6 transition-colors hover:border-white/25 sm:p-7";
  const content = (
    <>
      <span className="flex size-10 items-center justify-center rounded-xl bg-white/[0.06] ring-1 ring-white/10">
        <Icon className="size-5 text-[#7DFF86]" aria-hidden />
      </span>
      <span className="space-y-2">
        <span className="font-display block text-xl font-semibold tracking-tight">
          {title}
        </span>
        <span className="block text-[15px] leading-relaxed text-white/60">
          {body}
        </span>
      </span>
      <span className="mt-auto inline-flex items-center gap-1.5 text-sm font-medium text-[#7DFF86]">
        {action}
        <ArrowUpRight
          aria-hidden
          className="size-4 transition-transform group-hover:translate-x-0.5 group-hover:-translate-y-0.5"
        />
      </span>
    </>
  );
  return external ? (
    <a href={href} onPointerMove={trackSpot} className={className}>
      {content}
    </a>
  ) : (
    <Link href={href} onPointerMove={trackSpot} className={className}>
      {content}
    </Link>
  );
}

/** `/contact`: demo request, sales, support and docs entry points. */
export function ContactPage() {
  const { t, locale } = useLanding();
  const mail = (to: string, subject: string) =>
    `mailto:${to}?subject=${encodeURIComponent(subject)}`;

  return (
    <>
      <PageHero
        compact
        crumbs={[
          { href: landingHref(locale), label: t("landing.nav.home_short") },
          { label: t("landing.nav.contact") },
        ]}
        eyebrow={t("landing.contact.eyebrow")}
        title={t("landing.contact.title")}
        lead={t("landing.contact.lead")}
        actions={<LandingCta />}
      />
      <section className="pb-24 sm:pb-32">
        <ul className="mx-auto grid max-w-7xl gap-4 px-4 sm:px-6 md:grid-cols-3">
          <li>
            <Reveal className="h-full">
              <ContactCard
                external
                icon={CalendarClock}
                title={t("landing.contact.demo_title")}
                body={t("landing.contact.demo_body")}
                href={mail(contactEmail, t("landing.cta.demo_subject"))}
                action={contactEmail}
              />
            </Reveal>
          </li>
          <li>
            <Reveal delay={0.06} className="h-full">
              <ContactCard
                external
                icon={LifeBuoy}
                title={t("landing.contact.support_title")}
                body={t("landing.contact.support_body")}
                href={mail(
                  brand.supportEmail,
                  t("landing.contact.support_subject"),
                )}
                action={brand.supportEmail}
              />
            </Reveal>
          </li>
          <li>
            <Reveal delay={0.12} className="h-full">
              <ContactCard
                icon={BookOpenText}
                title={t("landing.contact.docs_title")}
                body={t("landing.contact.docs_body")}
                href={localizedHref(DOCS_PATH, locale)}
                action={t("landing.contact.docs_action")}
              />
            </Reveal>
          </li>
        </ul>
      </section>
    </>
  );
}
