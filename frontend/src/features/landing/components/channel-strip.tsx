"use client";

import { ChannelMark, LANDING_CHANNELS } from "./channels";
import { useLanding } from "./landing-context";
import { Reveal, SectionHeading } from "./section";

/**
 * The channels ClickFix brings into one inbox, as a slow marquee (still and
 * wrapped under reduced motion, paused on hover).
 */
export function ChannelStrip() {
  const { t } = useLanding();
  const labels = LANDING_CHANNELS.map((c) =>
    "label" in c ? c.label : t(c.labelKey),
  );
  const items = [...LANDING_CHANNELS, ...LANDING_CHANNELS];

  return (
    <section
      id="channels"
      aria-labelledby="channels-title"
      className="relative scroll-mt-20 border-y border-white/8 py-20 sm:py-24"
    >
      <div className="mx-auto max-w-7xl px-4 sm:px-6">
        <div id="channels-title">
          <SectionHeading
            eyebrow={t("landing.channels.eyebrow")}
            title={t("landing.channels.title")}
            lead={t("landing.channels.lead")}
          />
        </div>
      </div>
      <Reveal className="relative mt-12 overflow-hidden [mask-image:linear-gradient(to_right,transparent,#000_10%,#000_90%,transparent)]">
        <ul className="landing-marquee flex w-max gap-4 motion-reduce:w-full motion-reduce:flex-wrap motion-reduce:justify-center">
          {items.map((channel, i) => (
            <li
              key={`${channel.id}-${i}`}
              aria-hidden={i >= LANDING_CHANNELS.length}
              className="flex items-center gap-3 rounded-2xl border border-white/10 bg-white/[0.03] px-5 py-4 motion-reduce:aria-hidden:hidden"
            >
              <span
                aria-hidden
                className="flex size-9 items-center justify-center rounded-xl"
                style={{ background: `${channel.color}1f` }}
              >
                <ChannelMark id={channel.id} size="lg" />
              </span>
              <span className="font-display text-lg font-semibold tracking-tight whitespace-nowrap">
                {labels[i % LANDING_CHANNELS.length]}
              </span>
            </li>
          ))}
        </ul>
      </Reveal>
    </section>
  );
}
