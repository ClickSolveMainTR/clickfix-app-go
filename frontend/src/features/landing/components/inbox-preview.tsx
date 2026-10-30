"use client";

import { Bot, CheckCheck, Search, Sparkles, Timer } from "lucide-react";
import type { CSSProperties } from "react";

import { cn } from "@/lib/utils";

import { ChannelChip, channelById, type LandingChannelId } from "./channels";
import { useLanding } from "./landing-context";

type Row = {
  id: string;
  channel: LandingChannelId;
  name: string;
  sla: "ok" | "warn" | "done";
  unread?: boolean;
};

const ROWS: Row[] = [
  { id: "1", channel: "whatsapp", name: "Ayşe K.", sla: "ok", unread: true },
  { id: "2", channel: "instagram", name: "Mert D.", sla: "warn", unread: true },
  { id: "3", channel: "email", name: "Selin A.", sla: "ok" },
  { id: "4", channel: "widget", name: "Burak T.", sla: "done" },
];

function channelLabel(
  id: LandingChannelId,
  t: (key: string) => string,
): string {
  const channel = channelById(id);
  return "label" in channel ? channel.label : t(channel.labelKey);
}

/**
 * Hero visual: a stylised ClickFix inbox (conversation list, an open
 * WhatsApp thread, the AI agent drafting and an SLA timer). Static markup
 * with staggered reveals; the typing dots stop under reduced motion.
 */
export function InboxPreview({ className }: { className?: string }) {
  const { t } = useLanding();

  return (
    <div
      className={cn(
        "relative overflow-hidden rounded-2xl border border-white/10 bg-[#0D110E]/80 shadow-[0_40px_120px_-30px_rgb(0_0_0/0.9),0_0_0_1px_rgb(255_255_255/0.03)] backdrop-blur-xl",
        className,
      )}
      role="img"
      aria-label={t("landing.preview.label")}
    >
      {/* Window chrome */}
      <div className="flex items-center gap-2 border-b border-white/8 px-4 py-2.5">
        <span className="flex gap-1.5" aria-hidden>
          <span className="size-2.5 rounded-full bg-white/15" />
          <span className="size-2.5 rounded-full bg-white/15" />
          <span className="size-2.5 rounded-full bg-white/15" />
        </span>
        <span className="mx-auto flex h-6 w-1/2 items-center gap-1.5 rounded-md bg-white/[0.05] px-2 text-[10px] text-white/40">
          <Search className="size-3" aria-hidden />
          {t("landing.preview.search")}
        </span>
      </div>

      <div className="grid sm:grid-cols-[minmax(0,0.9fr)_minmax(0,1.3fr)]">
        {/* Conversation list */}
        <ul className="hidden border-e border-white/8 p-2 sm:block">
          {ROWS.map((row, i) => (
            <li
              key={row.id}
              style={{ "--d": `${0.5 + i * 0.08}s` } as CSSProperties}
              className={cn(
                "landing-fade mb-1 flex flex-col gap-1 rounded-lg px-2.5 py-2",
                i === 0 && "bg-white/[0.06] ring-1 ring-[#03FF13]/20",
              )}
            >
              <span className="flex items-center justify-between gap-2">
                <span className="truncate text-[12px] font-medium text-white/90">
                  {row.name}
                </span>
                {row.unread ? (
                  <span aria-hidden className="signal-dot size-1.5!" />
                ) : null}
              </span>
              <span className="truncate text-[11px] text-white/45">
                {t(`landing.preview.row_${row.id}`)}
              </span>
              <span className="flex items-center justify-between gap-1">
                <ChannelChip
                  id={row.channel}
                  label={channelLabel(row.channel, t)}
                />
                <span
                  className={cn(
                    "font-mono text-[10px] tabular-nums",
                    row.sla === "ok" && "text-[#7DFF86]",
                    row.sla === "warn" && "text-[#F2B544]",
                    row.sla === "done" && "text-white/35",
                  )}
                >
                  {row.sla === "done" ? (
                    <CheckCheck className="size-3" aria-hidden />
                  ) : (
                    t(`landing.preview.sla_${row.id}`)
                  )}
                </span>
              </span>
            </li>
          ))}
        </ul>

        {/* Open thread */}
        <div className="flex min-h-[19rem] flex-col gap-2.5 p-3 sm:p-4">
          <div className="flex items-center justify-between gap-2">
            <span className="truncate text-[12px] font-semibold text-white">
              Ayşe K.
            </span>
            <span className="flex items-center gap-1 rounded-full bg-[#03FF13]/10 px-2 py-0.5 text-[10px] text-[#7DFF86]">
              <Timer className="size-3" aria-hidden />
              {t("landing.preview.sla_badge")}
            </span>
          </div>
          <p
            style={{ "--d": "0.7s" } as CSSProperties}
            className="landing-rise max-w-[90%] rounded-xl rounded-tl-sm bg-white/[0.07] px-3 py-2 text-[12px] leading-relaxed text-white/85"
          >
            {t("landing.preview.customer")}
          </p>
          <div
            style={{ "--d": "1.1s" } as CSSProperties}
            className="landing-rise ms-auto max-w-[92%] rounded-xl rounded-tr-sm border border-[#03FF13]/25 bg-[#03FF13]/[0.07] px-3 py-2 text-[12px] leading-relaxed text-white/90"
          >
            <span className="font-accent mb-1 flex items-center gap-1.5 text-[9px] tracking-[0.16em] text-[#7DFF86] uppercase">
              <Bot className="size-3" aria-hidden />
              {t("landing.preview.ai")}
            </span>
            {t("landing.preview.reply")}
          </div>
          <div
            style={{ "--d": "1.6s" } as CSSProperties}
            className="landing-fade flex items-center gap-2 text-[10px] text-white/60"
          >
            <span className="landing-typing flex gap-0.5" aria-hidden>
              <span className="size-1 rounded-full bg-white/60" />
              <span className="size-1 rounded-full bg-white/60" />
              <span className="size-1 rounded-full bg-white/60" />
            </span>
            {t("landing.preview.typing")}
          </div>
          <div className="mt-auto rounded-xl border border-white/10 bg-white/[0.03] p-2.5">
            <p className="flex items-center gap-1.5 text-[10px] text-white/45">
              <Sparkles className="size-3 text-[#7DFF86]" aria-hidden />
              {t("landing.preview.copilot")}
            </p>
            <p className="mt-1.5 text-[11px] text-white/70">
              {t("landing.preview.copilot_text")}
              <span className="landing-caret ms-0.5 inline-block h-3 w-px translate-y-0.5 bg-[#03FF13]" />
            </p>
          </div>
        </div>
      </div>
    </div>
  );
}
