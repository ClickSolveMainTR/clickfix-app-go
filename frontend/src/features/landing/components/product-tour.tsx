"use client";

import { GrainGradient } from "@paper-design/shaders-react";
import {
  BellRing,
  Bot,
  BookOpenText,
  Clock3,
  Inbox,
  MessageSquareText,
  Plug,
  Smartphone,
  UserRoundCheck,
  type LucideIcon,
} from "lucide-react";
import type { ReactNode } from "react";

import { ShaderStage } from "@/components/brand/shader-stage";
import { brand } from "@/config/brand";
import { cn } from "@/lib/utils";

import { ChannelChip, LANDING_CHANNELS } from "./channels";
import { useLanding } from "./landing-context";
import { Reveal, SectionHeading, trackSpot } from "./section";

function TourCard({
  icon: Icon,
  title,
  body,
  children,
  className,
  delay = 0,
  backdrop,
}: {
  icon: LucideIcon;
  title: string;
  body: string;
  children?: ReactNode;
  className?: string;
  delay?: number;
  backdrop?: ReactNode;
}) {
  return (
    <Reveal delay={delay} className={cn("h-full", className)}>
      <article
        onPointerMove={trackSpot}
        className="landing-spot relative isolate flex h-full flex-col gap-5 overflow-hidden rounded-3xl border border-white/10 bg-[#0F1310] p-6 transition-colors hover:border-white/20 sm:p-7"
      >
        {backdrop}
        <span className="flex size-10 items-center justify-center rounded-xl bg-white/[0.06] ring-1 ring-white/10">
          <Icon className="size-5 text-[#7DFF86]" aria-hidden />
        </span>
        <div className="space-y-2">
          <h3 className="font-display text-xl font-semibold tracking-tight sm:text-2xl">
            {title}
          </h3>
          <p className="max-w-md text-sm leading-relaxed text-pretty text-white/60 sm:text-[15px]">
            {body}
          </p>
        </div>
        {children ? <div className="mt-auto">{children}</div> : null}
      </article>
    </Reveal>
  );
}

/** Product tour: the six things a support team gets, as a bento grid. */
export function ProductTour() {
  const { t } = useLanding();
  const channelLabel = (c: (typeof LANDING_CHANNELS)[number]) =>
    "label" in c ? c.label : t(c.labelKey);

  return (
    <section id="product" className="relative scroll-mt-20 py-24 sm:py-32">
      <div
        aria-hidden
        className="landing-gridlines pointer-events-none absolute inset-0 -z-10"
      />
      <div className="mx-auto max-w-7xl px-4 sm:px-6">
        <SectionHeading
          eyebrow={t("landing.tour.eyebrow")}
          title={t("landing.tour.title")}
          lead={t("landing.tour.lead")}
        />

        <div className="mt-14 grid gap-4 md:grid-cols-6">
          <TourCard
            className="md:col-span-4"
            icon={Inbox}
            title={t("landing.tour.inbox_title")}
            body={t("landing.tour.inbox_body")}
          >
            <ul className="grid gap-2 sm:grid-cols-2">
              {(["assign", "notes", "views", "merge"] as const).map((key) => (
                <li
                  key={key}
                  className="flex items-center gap-2 rounded-xl border border-white/8 bg-white/[0.03] px-3 py-2.5 text-sm text-white/75"
                >
                  <UserRoundCheck
                    className="size-4 shrink-0 text-white/40"
                    aria-hidden
                  />
                  {t(`landing.tour.inbox_${key}`)}
                </li>
              ))}
            </ul>
          </TourCard>

          <TourCard
            className="md:col-span-2 md:row-span-2"
            delay={0.08}
            icon={Bot}
            title={t("landing.tour.ai_title")}
            body={t("landing.tour.ai_body")}
            backdrop={
              <ShaderStage className="-z-10 bg-[#0F1310] opacity-70">
                {(active) => (
                  <GrainGradient
                    className="size-full"
                    colorBack="#0A0D0B"
                    colors={[...brand.shader.grain]}
                    softness={0.8}
                    intensity={0.35}
                    noise={0.35}
                    shape="corners"
                    speed={active ? 0.4 : 0}
                    maxPixelCount={900 * 900}
                  />
                )}
              </ShaderStage>
            }
          >
            <ol className="relative space-y-2.5">
              {(["kb", "draft", "guard", "handoff"] as const).map((key, i) => (
                <li
                  key={key}
                  className="flex items-start gap-3 rounded-xl bg-black/35 px-3 py-2.5 text-sm text-white/80 ring-1 ring-white/10 backdrop-blur"
                >
                  <span className="font-accent mt-0.5 text-xs text-[#7DFF86]">
                    0{i + 1}
                  </span>
                  {t(`landing.tour.ai_${key}`)}
                </li>
              ))}
            </ol>
          </TourCard>

          <TourCard
            className="md:col-span-2"
            delay={0.04}
            icon={Clock3}
            title={t("landing.tour.sla_title")}
            body={t("landing.tour.sla_body")}
          >
            <div className="space-y-2" aria-hidden>
              {[
                { w: "38%", tone: "bg-[#03FF13]" },
                { w: "72%", tone: "bg-[#F2B544]" },
                { w: "18%", tone: "bg-[#03FF13]" },
              ].map((bar, i) => (
                <div key={i} className="h-1.5 rounded-full bg-white/8">
                  <div
                    className={cn("h-full rounded-full", bar.tone)}
                    style={{ width: bar.w }}
                  />
                </div>
              ))}
            </div>
          </TourCard>

          <TourCard
            className="md:col-span-2"
            delay={0.08}
            icon={MessageSquareText}
            title={t("landing.tour.widget_title")}
            body={t("landing.tour.widget_body")}
          >
            <div className="flex items-end justify-end gap-2" aria-hidden>
              <span className="rounded-2xl rounded-br-sm bg-white/[0.07] px-3 py-2 text-xs text-white/75">
                {t("landing.tour.widget_bubble")}
              </span>
              <span className="flex size-10 items-center justify-center rounded-full bg-[#03FF13] text-black shadow-[0_0_24px_rgb(3_255_19/0.45)]">
                <MessageSquareText className="size-5" />
              </span>
            </div>
          </TourCard>

          <TourCard
            className="md:col-span-3"
            delay={0.04}
            icon={Smartphone}
            title={t("landing.tour.mobile_title")}
            body={t("landing.tour.mobile_body")}
          >
            <div className="flex items-center gap-3 rounded-2xl border border-white/10 bg-white/[0.04] p-3">
              <span className="flex size-9 items-center justify-center rounded-xl bg-[#0A0D0B] ring-1 ring-white/10">
                <BellRing className="size-4 text-[#7DFF86]" aria-hidden />
              </span>
              <span className="min-w-0 text-xs">
                <span className="block font-semibold text-white/90">
                  {t("landing.tour.mobile_push_title")}
                </span>
                <span className="block truncate text-white/55">
                  {t("landing.tour.mobile_push_body")}
                </span>
              </span>
            </div>
          </TourCard>

          <TourCard
            className="md:col-span-3"
            delay={0.08}
            icon={Plug}
            title={t("landing.tour.integrations_title")}
            body={t("landing.tour.integrations_body")}
          >
            <div className="flex flex-wrap gap-2">
              {LANDING_CHANNELS.map((c) => (
                <ChannelChip
                  key={c.id}
                  id={c.id}
                  label={channelLabel(c)}
                  className="px-2.5 py-1 text-xs"
                />
              ))}
              {(["api", "webhook", "openapi"] as const).map((key) => (
                <span
                  key={key}
                  className="inline-flex items-center gap-1.5 rounded-full border border-white/10 bg-white/[0.04] px-2.5 py-1 text-xs text-white/75"
                >
                  <BookOpenText className="size-3 text-white/45" aria-hidden />
                  {t(`landing.tour.integrations_${key}`)}
                </span>
              ))}
            </div>
          </TourCard>
        </div>
      </div>
    </section>
  );
}
