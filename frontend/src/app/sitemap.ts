import type { MetadataRoute } from "next";

import { routes } from "@/config/routes";
import { source } from "@/lib/docs/source";
import {
  COMPARE_PATH,
  CONTACT_PATH,
  DATA_DELETION_PATH,
  DOCS_PATH,
  FEATURES_PATH,
  LEGAL_DOCS,
  SECURITY_PATH,
} from "@/features/landing/constants";
import {
  FEATURE_PAGE_AREAS,
  featurePath,
} from "@/features/landing/content/areas";
import { siteBase } from "@/features/landing/lib/seo";

type Entry = MetadataRoute.Sitemap[number];

/** A Turkish page with its `?lang=en` alternate. */
function page(
  base: string,
  path: string,
  changeFrequency: Entry["changeFrequency"],
  priority: number,
): Entry {
  return {
    url: `${base}${path}`,
    changeFrequency,
    priority,
    alternates: {
      languages: {
        tr: `${base}${path}`,
        en: `${base}${path}?lang=en`,
        "x-default": `${base}${path}`,
      },
    },
  };
}

export default function sitemap(): MetadataRoute.Sitemap {
  const base = siteBase();
  return [
    // The public home is the same for signed-in and signed-out visitors.
    page(base, "/", "weekly", 1),
    {
      url: `${base}${routes.guest.signup}`,
      changeFrequency: "monthly",
      priority: 0.7,
    },
    page(base, FEATURES_PATH, "weekly", 0.9),
    ...FEATURE_PAGE_AREAS.map((area) =>
      page(base, featurePath(area), "monthly", 0.8),
    ),
    page(base, SECURITY_PATH, "monthly", 0.8),
    page(base, COMPARE_PATH, "monthly", 0.7),
    page(base, CONTACT_PATH, "yearly", 0.6),
    page(base, DOCS_PATH, "weekly", 0.8),
    ...source
      .getPages()
      .filter((doc) => doc.url !== DOCS_PATH)
      .map((doc) => page(base, doc.url, "monthly", 0.6)),
    ...LEGAL_DOCS.map((slug) => ({
      url: `${base}/legal/${slug}`,
      changeFrequency: "yearly" as const,
      priority: 0.3,
    })),
    {
      url: `${base}${DATA_DELETION_PATH}`,
      changeFrequency: "yearly",
      priority: 0.3,
    },
  ];
}
