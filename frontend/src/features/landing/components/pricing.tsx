"use client";

import { Check, Info, Minus } from "lucide-react";
import { useState } from "react";

import {
  CONTACT_PATH,
  contactEmail,
  SIGNUP_PATH,
} from "@/features/landing/constants";
import {
  PRICING_FEATURES,
  type PricingCatalog,
  type PricingCurrency,
  type PricingInterval,
  type PricingLine,
  type PricingPlan,
} from "@/features/landing/lib/pricing";
import { cn } from "@/lib/utils";

import { LandingCta } from "./landing-cta";
import { useLanding } from "./landing-context";
import { Reveal, SectionHeading } from "./section";

function formatMoney(minor: number, currency: string, locale: string) {
  return new Intl.NumberFormat(locale === "tr" ? "tr-TR" : "en-US", {
    style: "currency",
    currency,
    maximumFractionDigits: 0,
  }).format(minor / 100);
}

function limitValue(line: PricingLine, locale: string) {
  if (line.kind !== "limit" || line.value === "unlimited") return "";
  return line.feature === "storage.bytes"
    ? `${Math.round(line.value / 1024 ** 3)} GB`
    : new Intl.NumberFormat(locale === "tr" ? "tr-TR" : "en-US").format(
        line.value,
      );
}

function lineText(
  line: PricingLine,
  t: (key: string, params?: Record<string, string | number>) => string,
  locale: string,
) {
  if (line.kind === "flag") return t(`landing.pricing.feature.${line.feature}`);
  if (line.value === "unlimited")
    return t(`landing.pricing.feature_unlimited.${line.feature}`);
  return t(`landing.pricing.feature.${line.feature}`, {
    value: limitValue(line, locale),
  });
}

/** Sign-up link that carries the chosen price to the first organization. */
function signupHref(uuid: string | undefined) {
  return uuid ? `${SIGNUP_PATH}?plan=${encodeURIComponent(uuid)}` : SIGNUP_PATH;
}

/** Sales contact; the contact page when no address is configured. */
function salesHref(subject: string) {
  return contactEmail
    ? `mailto:${contactEmail}?subject=${encodeURIComponent(subject)}`
    : CONTACT_PATH;
}

function PlanCard({
  plan,
  interval,
  currency,
  featured,
}: {
  plan: PricingPlan;
  interval: PricingInterval;
  currency: PricingCurrency;
  featured: boolean;
}) {
  const { t, locale } = useLanding();
  const price = plan.prices[currency]?.[interval];
  const amount = price?.amount;
  const monthlyEquivalent =
    interval === "yearly" && amount ? Math.round(amount / 12) : amount;

  return (
    <article
      className={cn(
        "relative flex h-full flex-col gap-6 rounded-3xl border p-7",
        featured
          ? "border-[#03FF13]/45 bg-[#0F1A11] shadow-[0_30px_80px_-30px_rgb(3_255_19/0.35)]"
          : "border-white/10 bg-[#0F1310]",
      )}
    >
      {featured ? (
        <span className="font-accent absolute -top-3 left-7 rounded-full bg-[#03FF13] px-3 py-1 text-[10px] font-semibold tracking-[0.18em] text-black uppercase">
          {t("landing.pricing.popular")}
        </span>
      ) : null}
      <div className="space-y-2">
        <h3 className="font-display text-2xl font-semibold tracking-tight">
          {plan.name}
        </h3>
        {plan.description ? (
          <p className="text-sm text-white/55">{plan.description}</p>
        ) : null}
      </div>
      <div className="min-h-[4.5rem]">
        {monthlyEquivalent && price ? (
          <>
            <p className="flex items-baseline gap-1.5">
              <span className="font-display text-4xl font-semibold tracking-[-0.03em] tabular-nums">
                {formatMoney(monthlyEquivalent, currency, locale)}
              </span>
              <span className="text-sm text-white/50">
                {t("landing.pricing.per_month")}
              </span>
            </p>
            <p className="mt-1 text-xs text-white/45">
              {interval === "yearly"
                ? t("landing.pricing.billed_yearly", {
                    amount: formatMoney(amount ?? 0, currency, locale),
                  })
                : t("landing.pricing.billed_monthly")}
              {" · "}
              {price.taxInclusive
                ? t("landing.pricing.vat_included")
                : t("landing.pricing.vat_excluded", {
                    rate: price.vatBasisPoints / 100,
                  })}
            </p>
            {currency !== "TRY" ? (
              <p className="mt-1 text-[11px] text-white/40">
                {t("landing.pricing.fx_note")}
              </p>
            ) : null}
          </>
        ) : (
          <p className="font-display text-3xl font-semibold tracking-tight">
            {t("landing.pricing.custom")}
          </p>
        )}
      </div>
      <ul className="space-y-2.5 text-sm text-white/75">
        {plan.trialDays > 0 ? (
          <li className="flex items-start gap-2.5">
            <Check
              className="mt-0.5 size-4 shrink-0 text-[#7DFF86]"
              aria-hidden
            />
            {t("landing.pricing.trial", { days: plan.trialDays })}
          </li>
        ) : null}
        {plan.lines.map((line) => (
          <li key={line.feature} className="flex items-start gap-2.5">
            <Check
              className="mt-0.5 size-4 shrink-0 text-[#7DFF86]"
              aria-hidden
            />
            {lineText(line, t, locale)}
          </li>
        ))}
      </ul>
      <div className="mt-auto space-y-2">
        {monthlyEquivalent ? (
          <>
            <LandingCta
              size="sm"
              className="h-11 w-full"
              href={signupHref(price?.uuid)}
              label={
                plan.trialDays > 0
                  ? t("landing.pricing.start_trial")
                  : t("landing.pricing.buy")
              }
            />
            {plan.trialDays === 0 ? (
              <p className="text-center text-[11px] text-white/45">
                {t("landing.pricing.no_trial_hint")}
              </p>
            ) : null}
          </>
        ) : (
          <a
            href={salesHref(t("landing.pricing.sales_subject"))}
            className="inline-flex h-11 w-full items-center justify-center rounded-full border border-white/20 text-sm font-semibold text-white transition-colors hover:border-white/40"
          >
            {t("landing.pricing.contact_sales")}
          </a>
        )}
      </div>
    </article>
  );
}

function ComparisonTable({ plans }: { plans: PricingPlan[] }) {
  const { t, locale } = useLanding();
  const features = PRICING_FEATURES.filter((f) =>
    plans.some((p) => p.lines.some((l) => l.feature === f)),
  );
  if (features.length === 0 || plans.length < 2) return null;
  return (
    <div className="mt-16 overflow-x-auto rounded-3xl border border-white/10">
      <table className="w-full min-w-[640px] text-sm">
        <caption className="sr-only">{t("landing.pricing.compare")}</caption>
        <thead>
          <tr className="border-b border-white/10 text-left">
            <th scope="col" className="px-5 py-4 font-medium text-white/55">
              {t("landing.pricing.compare")}
            </th>
            {plans.map((p) => (
              <th
                key={p.code}
                scope="col"
                className="font-display px-5 py-4 text-base font-semibold"
              >
                {p.name}
              </th>
            ))}
          </tr>
        </thead>
        <tbody className="divide-y divide-white/8">
          {features.map((feature) => (
            <tr key={feature}>
              <th scope="row" className="px-5 py-3 font-normal text-white/70">
                {t(`landing.pricing.compare_row.${feature}`)}
              </th>
              {plans.map((p) => {
                const line = p.lines.find((l) => l.feature === feature);
                return (
                  <td key={p.code} className="px-5 py-3 tabular-nums">
                    {!line ? (
                      <Minus
                        className="size-4 text-white/25"
                        aria-label={t("landing.pricing.not_included")}
                      />
                    ) : line.kind === "flag" ? (
                      <Check
                        className="size-4 text-[#7DFF86]"
                        aria-label={t("landing.pricing.included")}
                      />
                    ) : line.value === "unlimited" ? (
                      t("landing.pricing.unlimited")
                    ) : (
                      limitValue(line, locale)
                    )}
                  </td>
                );
              })}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function Toggle<T extends string>({
  label,
  value,
  options,
  onChange,
  render,
}: {
  label: string;
  value: T;
  options: readonly T[];
  onChange: (v: T) => void;
  render: (v: T, active: boolean) => React.ReactNode;
}) {
  return (
    <div
      role="radiogroup"
      aria-label={label}
      className="flex shrink-0 rounded-full border border-white/12 bg-white/[0.03] p-1"
    >
      {options.map((option) => (
        <button
          key={option}
          type="button"
          role="radio"
          aria-checked={value === option}
          onClick={() => onChange(option)}
          className={cn(
            "rounded-full px-4 py-2 text-sm font-medium transition-colors",
            value === option
              ? "bg-white text-black"
              : "text-white/65 hover:text-white",
          )}
        >
          {render(option, value === option)}
        </button>
      ))}
    </div>
  );
}

/**
 * Pricing from `GET /v1/public/pricing` (server-fetched): monthly / yearly,
 * TRY / USD / EUR when plans are priced in them, a comparison table, and a
 * sign-up CTA per plan that carries its price to the first organization
 * (trial on that plan, or pay right away when it has no trial). While
 * nothing is published it shows the sample catalog, clearly labelled.
 */
export function Pricing({ catalog }: { catalog: PricingCatalog }) {
  const { t } = useLanding();
  const [currency, setCurrency] = useState<PricingCurrency>(
    catalog.currencies[0] ?? "TRY",
  );
  const hasYearly = catalog.plans.some((p) => p.prices[currency]?.yearly);
  const [interval, setInterval] = useState<PricingInterval>("monthly");
  const featuredIndex = catalog.plans.length >= 3 ? 1 : -1;

  return (
    <section
      id="pricing"
      className="relative scroll-mt-20 border-t border-white/8 py-24 sm:py-32"
    >
      <div className="mx-auto max-w-7xl px-4 sm:px-6">
        <div className="flex flex-col items-start justify-between gap-8 lg:flex-row lg:items-end">
          <SectionHeading
            eyebrow={t("landing.pricing.eyebrow")}
            title={t("landing.pricing.title")}
            lead={t("landing.pricing.lead")}
          />
          <div className="flex flex-wrap gap-3">
            {catalog.currencies.length > 1 ? (
              <Toggle
                label={t("landing.pricing.currency")}
                value={currency}
                options={catalog.currencies}
                onChange={setCurrency}
                render={(c) => c}
              />
            ) : null}
            {hasYearly ? (
              <Toggle
                label={t("landing.pricing.interval")}
                value={interval}
                options={["monthly", "yearly"] as const}
                onChange={setInterval}
                render={(value, active) => (
                  <>
                    {t(`landing.pricing.${value}`)}
                    {value === "yearly" ? (
                      <span
                        className={cn(
                          "ms-2 rounded-full px-1.5 py-0.5 text-[10px]",
                          active
                            ? "bg-[#0A0D0B] text-[#7DFF86]"
                            : "bg-[#03FF13]/15 text-[#7DFF86]",
                        )}
                      >
                        {t("landing.pricing.yearly_badge")}
                      </span>
                    ) : null}
                  </>
                )}
              />
            ) : null}
          </div>
        </div>

        <div
          className={cn(
            "mt-14 grid gap-4",
            catalog.plans.length >= 3 ? "lg:grid-cols-3" : "md:grid-cols-2",
          )}
        >
          {catalog.plans.map((plan, i) => (
            <Reveal key={plan.code} delay={i * 0.08}>
              <PlanCard
                plan={plan}
                interval={interval}
                currency={currency}
                featured={i === featuredIndex}
              />
            </Reveal>
          ))}
        </div>

        <ComparisonTable plans={catalog.plans} />

        <div className="mt-8 flex flex-col items-start justify-between gap-4 rounded-3xl border border-white/10 bg-white/[0.02] p-6 sm:flex-row sm:items-center">
          <div>
            <p className="font-display text-lg font-semibold">
              {t("landing.pricing.enterprise_title")}
            </p>
            <p className="text-sm text-white/55">
              {t("landing.pricing.enterprise_lead")}
            </p>
          </div>
          <a
            href={salesHref(t("landing.pricing.sales_subject"))}
            className="inline-flex h-11 shrink-0 items-center justify-center rounded-full border border-white/20 px-5 text-sm font-semibold text-white transition-colors hover:border-white/40"
          >
            {t("landing.pricing.contact_sales")}
          </a>
        </div>

        {catalog.source === "fallback" ? (
          <p className="mt-6 flex items-start gap-2 text-xs text-white/50">
            <Info className="mt-0.5 size-3.5 shrink-0" aria-hidden />
            {t("landing.pricing.sample_note")}
          </p>
        ) : null}
      </div>
    </section>
  );
}
