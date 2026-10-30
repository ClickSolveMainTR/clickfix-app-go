import { createMDX } from "fumadocs-mdx/next";
import legacyRedirects from "./legacy-redirects.json";
import { withSentryConfig } from "@sentry/nextjs/config";
import type { NextConfig } from "next";

/**
 * Baseline security headers for every route. A full script CSP is not set
 * here (Next inline bootstrap scripts would need nonces); `frame-ancestors`
 * and `base-uri` restrictions are safe to enforce globally.
 */
const securityHeaders = [
  {
    key: "Content-Security-Policy",
    value: "frame-ancestors 'self'; base-uri 'self'; object-src 'none'",
  },
  { key: "X-Frame-Options", value: "SAMEORIGIN" },
  { key: "X-Content-Type-Options", value: "nosniff" },
  { key: "Referrer-Policy", value: "strict-origin-when-cross-origin" },
  {
    key: "Permissions-Policy",
    value: "camera=(self), microphone=(self), geolocation=(), payment=()",
  },
  ...(process.env.NODE_ENV === "production"
    ? [
        {
          key: "Strict-Transport-Security",
          value: "max-age=31536000; includeSubDomains",
        },
      ]
    : []),
];

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
  },
  async headers() {
    return [{ source: "/:path*", headers: securityHeaders }];
  },
  async redirects() {
    return [
      ...settingsRedirects,
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
