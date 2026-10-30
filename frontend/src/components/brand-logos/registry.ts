import type { ComponentType } from "react";
import {
  siCaldotcom,
  siCalendly,
  siGmail,
  siGoogledocs,
  siGooglecalendar,
  siGooglemeet,
  siGoogletagmanager,
  siHubspot,
  siIntercom,
  siJira,
  siLinear,
  siMake,
  siMetabase,
  siN8n,
  siNextdotjs,
  siOpenstreetmap,
  siReact,
  siResend,
  siShopify,
  siStripe,
  siUptimekuma,
  siVuedotjs,
  siWoocommerce,
  siWordpress,
  siZapier,
  siZendesk,
  siZoom,
} from "simple-icons";

import { AppleLogo } from "./apple";
import type { BrandLogoProps } from "./brand-svg";
import { FacebookLogo } from "./facebook";
import { GitHubLogo } from "./github";
import { GoogleLogo } from "./google";
import { GoogleChatLogo } from "./google-chat";
import { InstagramLogo } from "./instagram";
import { IyzicoLogo } from "./iyzico";
import { MessengerLogo } from "./messenger";
import { MetaLogo } from "./meta";
import { MicrosoftTeamsLogo } from "./microsoft-teams";
import { NetgsmLogo } from "./netgsm";
import { PaytrLogo } from "./paytr";
import { simpleIconLogo } from "./simple-icon";
import { SlackLogo } from "./slack";
import { TelegramLogo } from "./telegram";
import { TwilioLogo } from "./twilio";
import { WhatsAppLogo } from "./whatsapp";

/**
 * Every external company logo the UI shows, by slug. Slugs match the values
 * the API uses (channel types, OAuth providers, payment providers, SMS/voice
 * providers, widget install platforms, gallery app ids). Single-colour marks
 * come from the `simple-icons` package; sources and guideline notes:
 * docs/brand-logos.md.
 */
export const BRAND_LOGOS = {
  whatsapp: WhatsAppLogo,
  instagram: InstagramLogo,
  messenger: MessengerLogo,
  telegram: TelegramLogo,
  google_chat: GoogleChatLogo,
  facebook: FacebookLogo,
  meta: MetaLogo,
  google: GoogleLogo,
  apple: AppleLogo,
  github: GitHubLogo,
  paytr: PaytrLogo,
  iyzico: IyzicoLogo,
  // Channels and providers
  slack: SlackLogo,
  teams: MicrosoftTeamsLogo,
  twilio: TwilioLogo,
  netgsm: NetgsmLogo,
  resend: simpleIconLogo(siResend),
  gmail: simpleIconLogo(siGmail),
  // Automation tools
  zapier: simpleIconLogo(siZapier),
  make: simpleIconLogo(siMake),
  n8n: simpleIconLogo(siN8n),
  // Sidebar app gallery
  calendly: simpleIconLogo(siCalendly),
  uptime_kuma: simpleIconLogo(siUptimekuma),
  google_docs: simpleIconLogo(siGoogledocs),
  metabase: simpleIconLogo(siMetabase),
  openstreetmap: simpleIconLogo(siOpenstreetmap),
  // Widget install platforms
  nextjs: simpleIconLogo(siNextdotjs),
  react: simpleIconLogo(siReact),
  vue: simpleIconLogo(siVuedotjs),
  wordpress: simpleIconLogo(siWordpress),
  woocommerce: simpleIconLogo(siWoocommerce),
  shopify: simpleIconLogo(siShopify),
  gtm: simpleIconLogo(siGoogletagmanager),
  // Scheduling, meetings, product tools, commerce
  google_calendar: simpleIconLogo(siGooglecalendar),
  google_meet: simpleIconLogo(siGooglemeet),
  zoom: simpleIconLogo(siZoom),
  cal_com: simpleIconLogo(siCaldotcom),
  jira: simpleIconLogo(siJira),
  linear: simpleIconLogo(siLinear),
  stripe: simpleIconLogo(siStripe),
  hubspot: simpleIconLogo(siHubspot),
  zendesk: simpleIconLogo(siZendesk),
  intercom: simpleIconLogo(siIntercom),
} satisfies Record<string, ComponentType<BrandLogoProps>>;

export type BrandSlug = keyof typeof BRAND_LOGOS;

/** Logos wider than tall: size them by height (`h-4 w-auto`). */
export const WORDMARK_SLUGS: ReadonlySet<BrandSlug> = new Set([
  "paytr",
  "iyzico",
]);

export function isBrandSlug(value: string): value is BrandSlug {
  return Object.hasOwn(BRAND_LOGOS, value);
}
