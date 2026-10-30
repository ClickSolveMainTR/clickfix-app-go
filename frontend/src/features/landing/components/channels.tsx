import { BrandLogo, isBrandSlug } from "@/components/brand-logos";

/**
 * Channels shown on the landing page. Brand channels show their original
 * logo; `color` tints the tile behind it (and is the dot of the others).
 */
export const LANDING_CHANNELS = [
  { id: "whatsapp", label: "WhatsApp", color: "#25D366" },
  { id: "instagram", label: "Instagram", color: "#E1306C" },
  { id: "messenger", label: "Messenger", color: "#0084FF" },
  { id: "telegram", label: "Telegram", color: "#2AABEE" },
  { id: "google_chat", label: "Google Chat", color: "#00AC47" },
  { id: "slack", label: "Slack", color: "#E01E5A" },
  { id: "teams", label: "Microsoft Teams", color: "#7B83EB" },
  { id: "email", labelKey: "landing.channels.email", color: "#C9D1CB" },
  { id: "sms", labelKey: "landing.channels.sms", color: "#F2B544" },
  { id: "widget", labelKey: "landing.channels.widget", color: "#03FF13" },
] as const;

export type LandingChannelId = (typeof LANDING_CHANNELS)[number]["id"];

export function channelById(id: LandingChannelId) {
  return LANDING_CHANNELS.find((c) => c.id === id)!;
}

export function ChannelChip({
  id,
  label,
  className = "",
}: {
  id: LandingChannelId;
  label: string;
  className?: string;
}) {
  return (
    <span
      className={`inline-flex items-center gap-1.5 rounded-full border border-white/10 bg-white/[0.04] px-2 py-0.5 text-[10px] font-medium text-white/75 ${className}`}
    >
      <ChannelMark id={id} size="sm" />
      {label}
    </span>
  );
}

/** Original logo of a brand channel (white variants: the landing is dark). */
export function ChannelMark({
  id,
  size,
}: {
  id: LandingChannelId;
  size: "sm" | "lg";
}) {
  if (isBrandSlug(id)) {
    return (
      <BrandLogo
        slug={id}
        tone="dark"
        decorative
        className={size === "sm" ? "size-3" : "size-5"}
      />
    );
  }
  const { color } = channelById(id);
  return (
    <span
      aria-hidden
      className={
        size === "sm" ? "size-1.5 rounded-full" : "size-2.5 rounded-full"
      }
      style={{
        background: color,
        boxShadow: size === "lg" ? `0 0 12px ${color}` : undefined,
      }}
    />
  );
}
