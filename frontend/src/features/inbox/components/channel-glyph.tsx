import {
  FlaskConical,
  Globe,
  Mail,
  MessagesSquare,
  Phone,
  Smartphone,
  Webhook,
  type LucideIcon,
} from "lucide-react";

import { BrandLogo, isBrandSlug } from "@/components/brand-logos";
import { cn } from "@/lib/utils";

/** Channel types that are not an external company. */
const GLYPHS: Record<string, { icon: LucideIcon; tone: string }> = {
  email: { icon: Mail, tone: "text-violet-600" },
  widget: { icon: Globe, tone: "text-foreground" },
  sms: { icon: Smartphone, tone: "text-amber-600" },
  voice: { icon: Phone, tone: "text-teal-600" },
  api: { icon: Webhook, tone: "text-slate-600" },
  test: { icon: FlaskConical, tone: "text-muted-foreground" },
};

/**
 * Small icon of a channel type (original logo for brand channels). Pass
 * `title` when no visible label sits next to it.
 */
export function ChannelGlyph({
  type,
  className,
  title,
}: {
  type: string;
  className?: string;
  title?: string;
}) {
  if (isBrandSlug(type)) {
    return (
      <BrandLogo
        slug={type}
        className={className}
        title={title}
        decorative={!title}
      />
    );
  }
  const g = GLYPHS[type] ?? {
    icon: MessagesSquare,
    tone: "text-muted-foreground",
  };
  const Icon = g.icon;
  return <Icon className={cn(g.tone, className)} aria-hidden />;
}
