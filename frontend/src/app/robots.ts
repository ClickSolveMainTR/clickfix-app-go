import type { MetadataRoute } from "next";

import { brand } from "@/config/brand";

/**
 * Workspace (app shell) sections: signed-in only, never useful in search.
 * Kept in sync with the top-level folders of `app/(cms)` and `(setup)`.
 */
const APP_PATHS = [
  "/dashboard",
  "/apps",
  "/billing",
  "/calendar",
  "/channels",
  "/coaching",
  "/companies",
  "/contacts",
  "/forum",
  "/inbox",
  "/setup",
  "/objects",
  "/oncall",
  "/profile",
  "/replies",
  "/reports",
  "/rules",
  "/settings",
  "/team-status",
  "/teams",
  "/video",
  "/widget",
  "/workflows",
  "/workforce",
  "/getting-started",
];

export default function robots(): MetadataRoute.Robots {
  const base = (process.env.SITE_URL || brand.siteUrl).replace(/\/$/, "");
  return {
    rules: [
      {
        userAgent: "*",
        allow: ["/"],
        disallow: [
          "/api/",
          "/platform/",
          "/login",
          "/book/manage/",
          "/docs/search",
          "/docs/og/",
          "/invite/",
          "/share/",
          "/r/",
          "/unauthorized",
          "/forbidden",
          ...APP_PATHS,
        ],
      },
    ],
    sitemap: `${base}/sitemap.xml`,
    host: base,
  };
}
