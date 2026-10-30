"use client";

import Image from "next/image";
import { Check, Minus } from "lucide-react";

import {
  COMPETITORS,
  compareGroups,
  type CompareValue,
} from "@/features/landing/content/compare";
import { landingHref } from "@/features/landing/lib/locale";
import { cn } from "@/lib/utils";

import { FinalCta } from "./final-cta";
import { LandingCta } from "./landing-cta";
import { useLanding } from "./landing-context";
import { PageHero } from "./page-hero";
import { Reveal } from "./section";

function Cell({ value, strong }: { value: CompareValue; strong?: boolean }) {
  const { t } = useLanding();
  if (typeof value === "object") {
    return <span className="text-xs text-white/65">{value.text}</span>;
  }
  if (value === "yes") {
    return (
      <span
        className={cn(
          "inline-flex size-6 items-center justify-center rounded-full",
          strong ? "bg-[#03FF13] text-black" : "bg-white/10 text-white/80",
        )}
      >
        <Check className="size-3.5" aria-hidden />
        <span className="sr-only">{t("landing.compare.value_yes")}</span>
      </span>
    );
  }
  if (value === "none") {
    return (
      <span className="inline-flex text-white/30">
        <Minus className="size-4" aria-hidden />
        <span className="sr-only">{t("landing.compare.value_none")}</span>
      </span>
    );
  }
  return (
    <span className="rounded-full border border-white/12 px-2 py-0.5 text-[11px] whitespace-nowrap text-white/65">
      {t(`landing.compare.value_${value}`)}
    </span>
  );
}

/** `/compare`: ClickFix next to the global suites, with sources. */
export function ComparePage() {
  const { t, locale } = useLanding();
  const groups = compareGroups(locale);

  return (
    <>
      <PageHero
        compact
        crumbs={[
          { href: landingHref(locale), label: t("landing.nav.home_short") },
          { label: t("landing.nav.compare") },
        ]}
        eyebrow={t("landing.compare.eyebrow")}
        title={t("landing.compare.title")}
        lead={t("landing.compare.lead")}
        actions={<LandingCta />}
      />

      <section className="pb-20 sm:pb-28">
        <div className="mx-auto max-w-6xl px-4 sm:px-6">
          <Reveal>
            <div className="overflow-x-auto rounded-3xl border border-white/10 bg-[#0F1310]">
              <table className="w-full min-w-[640px] border-collapse text-left text-sm">
                <caption className="sr-only">
                  {t("landing.compare.caption")}
                </caption>
                <thead>
                  <tr className="border-b border-white/10">
                    <th
                      scope="col"
                      className="w-[40%] px-5 py-4 font-medium text-white/50"
                    >
                      {t("landing.compare.capability")}
                    </th>
                    <th
                      scope="col"
                      className="font-display bg-[#03FF13]/[0.06] px-4 py-4 text-center text-base font-semibold text-white"
                    >
                      ClickFix
                    </th>
                    {COMPETITORS.map((name) => (
                      <th
                        key={name}
                        scope="col"
                        className="px-4 py-4 text-center font-medium text-white/70"
                      >
                        <span className="inline-flex flex-col items-center gap-2">
                          {name !== "Freshdesk" && (
                            <Image
                              src={`/brands/${name.toLowerCase()}.svg`}
                              alt=""
                              width={28}
                              height={28}
                              sizes="28px"
                              className="size-7 invert"
                            />
                          )}
                          <span
                            className={
                              name === "Freshdesk"
                                ? "font-display font-semibold"
                                : undefined
                            }
                          >
                            {name}
                          </span>
                        </span>
                      </th>
                    ))}
                  </tr>
                </thead>
                {groups.map((group) => (
                  <tbody key={group.title}>
                    <tr>
                      <th
                        scope="colgroup"
                        colSpan={2 + COMPETITORS.length}
                        className="font-accent bg-white/[0.02] px-5 pt-5 pb-2 text-[10px] font-semibold tracking-[0.22em] text-[#7DFF86] uppercase"
                      >
                        {group.title}
                      </th>
                    </tr>
                    {group.rows.map((row) => (
                      <tr key={row.label} className="border-t border-white/6">
                        <th
                          scope="row"
                          className="px-5 py-3.5 font-normal text-white/80"
                        >
                          {row.label}
                        </th>
                        <td className="bg-[#03FF13]/[0.06] px-4 py-3.5 text-center">
                          <Cell value={row.clickfix} strong />
                        </td>
                        {row.others.map((value, i) => (
                          <td
                            key={COMPETITORS[i]}
                            className="px-4 py-3.5 text-center"
                          >
                            <Cell value={value} />
                          </td>
                        ))}
                      </tr>
                    ))}
                  </tbody>
                ))}
              </table>
            </div>
          </Reveal>

          <dl className="mt-6 grid gap-x-6 gap-y-2 text-xs text-white/55 sm:grid-cols-2">
            {(["plan", "addon", "none"] as const).map((key) => (
              <div key={key} className="flex gap-2">
                <dt className="shrink-0">
                  <Cell value={key} />
                </dt>
                <dd>{t(`landing.compare.legend_${key}`)}</dd>
              </div>
            ))}
          </dl>
          <p className="mt-6 text-xs text-white/55">
            {locale === "tr"
              ? "Ürün adları ve logolar ilgili sahiplerinin ticari markalarıdır. Yalnızca ürünleri tanımlamak için kullanılmıştır; bir ortaklık veya onay anlamına gelmez."
              : "Product names and logos are trademarks of their respective owners, used only to identify products. No affiliation or endorsement is implied."}
          </p>
          <p className="mt-6 max-w-3xl text-xs leading-relaxed text-white/45">
            {t("landing.compare.sources")}
          </p>
          <p className="mt-2 max-w-3xl text-xs leading-relaxed text-white/45">
            {t("landing.compare.trademarks")}
          </p>
        </div>
      </section>

      <FinalCta />
    </>
  );
}
