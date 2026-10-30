"use client";

import { ArrowUpRight } from "lucide-react";
import Link from "next/link";

import { featureCopy } from "@/features/landing/content/features";
import {
  FEATURE_AREAS,
  featureHref,
  type FeatureSlug,
} from "@/features/landing/content/areas";
import { cn } from "@/lib/utils";

import { useLanding } from "./landing-context";
import { Reveal, trackSpot } from "./section";

/**
 * Every product area as a linked card (home "Platform" section, the
 * features index and "other areas" under a feature page).
 */
export function ModulesGrid({
  exclude,
  className,
}: {
  exclude?: FeatureSlug;
  className?: string;
}) {
  const { locale } = useLanding();
  const copy = featureCopy(locale);
  const areas = FEATURE_AREAS.filter((area) => area.slug !== exclude);

  return (
    <ul
      className={cn(
        "grid gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4",
        className,
      )}
    >
      {areas.map((area, i) => {
        const Icon = area.icon;
        const text = copy[area.slug];
        return (
          <li key={area.slug}>
            <Reveal delay={(i % 4) * 0.05} className="h-full">
              <Link
                href={featureHref(area, locale)}
                onPointerMove={trackSpot}
                className="landing-spot group relative flex h-full flex-col gap-4 rounded-2xl border border-white/10 bg-[#0F1310] p-5 transition-colors hover:border-white/25"
              >
                <span className="flex items-center justify-between">
                  <span className="flex size-10 items-center justify-center rounded-xl bg-white/[0.06] ring-1 ring-white/10 transition-colors group-hover:bg-[#03FF13]/10 group-hover:ring-[#03FF13]/30">
                    <Icon className="size-5 text-[#7DFF86]" aria-hidden />
                  </span>
                  <ArrowUpRight
                    aria-hidden
                    className="size-4 text-white/30 transition-[color,transform] group-hover:translate-x-0.5 group-hover:-translate-y-0.5 group-hover:text-white"
                  />
                </span>
                <span className="space-y-1.5">
                  <span className="font-display block text-lg font-semibold tracking-tight">
                    {text.name}
                  </span>
                  <span className="block text-sm leading-relaxed text-white/55">
                    {text.summary}
                  </span>
                </span>
              </Link>
            </Reveal>
          </li>
        );
      })}
    </ul>
  );
}
