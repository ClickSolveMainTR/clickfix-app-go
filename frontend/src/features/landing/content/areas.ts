import {
  Activity,
  BarChart3,
  BookOpenText,
  Bot,
  Building2,
  Code2,
  GitBranch,
  Inbox,
  ShieldCheck,
  Smartphone,
  Video,
  Workflow,
  type LucideIcon,
} from "lucide-react";

import type { AppLocale } from "@/config/i18n";
import { FEATURES_PATH, SECURITY_PATH } from "@/features/landing/constants";
import { localizedHref } from "@/features/landing/lib/locale";

/**
 * Product areas of the marketing site. Each has a page under
 * `/features/<slug>` (copy in `features.<locale>.ts`), except security,
 * whose page is `/security`. `docs` are `/docs/<slug>` guides linked from
 * the page (missing ones are skipped).
 */
export const FEATURE_AREAS = [
  {
    slug: "inbox",
    icon: Inbox,
    docs: ["inbox", "channels", "meta", "email", "web-widget"],
  },
  { slug: "ai", icon: Bot, docs: ["ai"] },
  { slug: "knowledge-base", icon: BookOpenText, docs: ["knowledge-base"] },
  {
    slug: "appointments-video",
    icon: Video,
    docs: ["appointments", "video-rooms"],
  },
  { slug: "customer-relationships", icon: Building2, docs: ["inbox"] },
  {
    slug: "engineering",
    icon: GitBranch,
    docs: [],
  },
  { slug: "automation", icon: Workflow, docs: ["automation"] },
  { slug: "analytics", icon: BarChart3, docs: [] },
  { slug: "status-page", icon: Activity, docs: [] },
  {
    slug: "security",
    icon: ShieldCheck,
    docs: ["data-protection", "sso-scim"],
    href: SECURITY_PATH,
  },
  { slug: "mobile", icon: Smartphone, docs: ["getting-started"] },
  {
    slug: "developers",
    icon: Code2,
    docs: ["api", "webhooks", "mcp-integrations"],
  },
] as const satisfies readonly {
  slug: string;
  icon: LucideIcon;
  docs: readonly string[];
  href?: string;
}[];

export type FeatureArea = (typeof FEATURE_AREAS)[number];
export type FeatureSlug = FeatureArea["slug"];

/** Areas with their own `/features/<slug>` page. */
export const FEATURE_PAGE_AREAS = FEATURE_AREAS.filter(
  (area) => !("href" in area),
);

export function findFeatureArea(slug: string) {
  return FEATURE_PAGE_AREAS.find((area) => area.slug === slug);
}

export function featurePath(area: FeatureArea) {
  return "href" in area ? area.href : `${FEATURES_PATH}/${area.slug}`;
}

export function featureHref(area: FeatureArea, locale: AppLocale) {
  return localizedHref(featurePath(area), locale);
}
