import {
  FlaskConical,
  Mail,
  MessagesSquare,
  Phone,
  Smartphone,
  Webhook,
  type LucideIcon,
} from "lucide-react";

import { BrandLogo, isBrandSlug } from "@/components/brand-logos";

/** Channel types that are not an external company. */
const icons: Record<string, LucideIcon> = {
  email: Mail,
  widget: MessagesSquare,
  sms: Smartphone,
  voice: Phone,
  api: Webhook,
  test: FlaskConical,
};

/**
 * Icon of a channel type: the company's original logo for brand channels
 * (WhatsApp, Instagram, Messenger, Telegram, Slack, Teams, Google Chat), a
 * generic glyph otherwise.
 */
export function ChannelIcon({
  type,
  className,
}: {
  type: string;
  className?: string;
}) {
  if (isBrandSlug(type)) {
    return <BrandLogo slug={type} className={className} decorative />;
  }
  const Icon = icons[type] ?? MessagesSquare;
  return <Icon className={className} aria-hidden />;
}
