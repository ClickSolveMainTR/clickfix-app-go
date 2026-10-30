import type { AppLocale } from "@/config/i18n";

import type { FeatureSlug } from "./areas";
import { featuresEn } from "./features.en";
import { featuresTr } from "./features.tr";

/**
 * Long-form copy of a product area page. Kept in typed modules instead of
 * the flat locale JSON: each area is a block of prose with lists, and the
 * compiler checks that both languages cover every area. Every claim must
 * match what the product does on `main` (CF-133).
 */
export type FeatureCopy = {
  /** Short name (nav, footer, cards). */
  name: string;
  /** One line on the area card. */
  summary: string;
  eyebrow: string;
  /** Page H1. */
  title: string;
  lead: string;
  /** Meta description (<= 160 chars). */
  description: string;
  /** Short labels floating in the hero visual. */
  chips: string[];
  highlights: { title: string; body: string }[];
  capabilities: string[];
};

export type FeatureCatalog = Record<FeatureSlug, FeatureCopy>;

const CATALOG: Record<AppLocale, FeatureCatalog> = {
  tr: featuresTr,
  en: featuresEn,
};

export function featureCopy(locale: AppLocale): FeatureCatalog {
  return CATALOG[locale];
}
