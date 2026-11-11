import { createMDX } from "fumadocs-mdx/next";
import legacyRedirects from "./legacy-redirects.json";
import { withSentryConfig } from "@sentry/nextjs/config";
import type { NextConfig } from "next";

import { headerRules } from "./src/config/security-headers";

/**
 * Platform admin pages moved under the System Settings shell
 * (`/platform/settings/*`). Old URLs (bookmarks, emailed links, stored
 * notification action URLs) keep working via permanent (308) redirects.
 */
const settingsRedirects = [
  { source: "/platform/roles", destination: "/platform/settings/roles" },
  {
    source: "/platform/roles/:path*",
    destination: "/platform/settings/roles/:path*",
  },
  { source: "/platform/activity", destination: "/platform/settings/activity" },
  { source: "/platform/logs", destination: "/platform/settings/logs" },
  { source: "/platform/storage", destination: "/platform/settings/storage" },
  { source: "/platform/access", destination: "/platform/settings/access" },
  {
    source: "/platform/auth/settings",
    destination: "/platform/settings/auth",
  },
  {
    source: "/platform/integrations",
    destination: "/platform/settings/integrations",
  },
  {
    source: "/platform/integrations/:provider(github|google|facebook|apple)",
    destination: "/platform/settings/integrations?tab=:provider",
  },
  { source: "/platform/imports", destination: "/platform/settings/imports" },
  {
    source: "/platform/imports/:path*",
    destination: "/platform/settings/imports/:path*",
  },
  { source: "/platform/exports", destination: "/platform/settings/exports" },
  {
    source: "/platform/exports/:path*",
    destination: "/platform/settings/exports/:path*",
  },
].map((rule) => ({ ...rule, permanent: true }));

/**
 * Docs pages that moved when the guide was split into step pages and the
 * SDK page joined it (CF-208); npm / Packagist READMEs link the old URLs.
 * The query (`?lang=en`) is carried over.
 */
const docsRedirects = [
  { source: "/docs/sdks", destination: "/docs/integration-guide/sdks" },
].map((rule) => ({ ...rule, permanent: true }));

/**
 * Common spellings of the legal document slugs (`LEGAL_DOCS`) people type or
 * link to (CF-135); the query (`?lang=en`) is carried over.
 */
const nextConfig: NextConfig = {
  output: "standalone",
  turbopack: { root: process.env.NEXT_TURBOPACK_ROOT ?? process.cwd() },
  /**
   * Metadata (title, description, canonical, hreflang, Open Graph) is
   * rendered into <head> for every client, not streamed into <body> for
   * non-bot user agents: link-preview scrapers and SEO tools that are not on
   * Next's bot list see it too (CF-144). Marketing metadata reads cached
   * data, so this costs no measurable TTFB.
   */
  htmlLimitedBots: /.*/,
  /**
   * Customer docs are Markdown read at request time (`features/docs`); the
   * standalone server only ships traced files, so include the folder.
   */
  outputFileTracingIncludes: {
    "/docs": ["./content/docs/**/*", "./content/api/public-api.yaml"],
    "/docs/[[...slug]]": [
      "./content/docs/**/*",
      "./content/api/public-api.yaml",
    ],
    "/docs/search": ["./content/docs/**/*", "./content/api/public-api.yaml"],
    "/features/[slug]": [
      "./content/docs/**/*",
      "./content/api/public-api.yaml",
    ],
    "/security": ["./content/docs/**/*", "./content/api/public-api.yaml"],
    "/en/docs": ["./content/docs/**/*", "./content/api/public-api.yaml"],
    "/en/docs/[[...slug]]": [
      "./content/docs/**/*",
      "./content/api/public-api.yaml",
    ],
    "/sitemap.xml": ["./content/docs/**/*", "./content/api/public-api.yaml"],
    "/llms.txt": ["./content/docs/**/*", "./content/api/public-api.yaml"],
    "/llms-full.txt": ["./content/docs/**/*", "./content/api/public-api.yaml"],
    "/en/llms.txt": ["./content/docs/**/*", "./content/api/public-api.yaml"],
    "/en/llms-full.txt": [
      "./content/docs/**/*",
      "./content/api/public-api.yaml",
    ],
    "/llms.mdx/[locale]/[[...slug]]": [
      "./content/docs/**/*",
      "./content/api/public-api.yaml",
    ],
  },
  async rewrites() {
    // Raw Markdown of an English docs page without the query form (CF-208);
    // the proxy handles `/docs/<slug>.md` and `?lang=en`.
    return [
      { source: "/en/docs/index.md", destination: "/llms.mdx/en/_index" },
      { source: "/en/docs/:path*.md", destination: "/llms.mdx/en/:path*" },
    ];
  },
  async headers() {
    return headerRules(process.env.NODE_ENV === "production");
  },
  async redirects() {
    return [
      ...settingsRedirects,
      ...docsRedirects,
      ...legacyRedirects.map((rule) => ({ ...rule, permanent: true })),
    ];
  },
};

/**
 * The error tracker is a self-hosted Sentry-protocol endpoint without the
 * sentry-cli API, so nothing is uploaded at build time: no source maps, no
 * release creation, no telemetry. The build needs no Sentry env or token;
 * DSNs are read at runtime (SENTRY_DSN).
 */
export default createMDX()(
  withSentryConfig(nextConfig, {
    silent: true,
    telemetry: false,
    authToken: undefined,
    sourcemaps: { disable: true },
    release: { create: false, finalize: false },
  }),
);
