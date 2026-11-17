/**
 * Response header rules of the web app (`next.config.ts`, `headers()`), kept
 * here so they can be tested. Free of imports (`node --test`).
 */

export type HeaderRule = {
  source: string;
  headers: { key: string; value: string }[];
};

/**
 * Baseline security headers for every route. A full script CSP is not set
 * here (Next inline bootstrap scripts would need nonces); `frame-ancestors`
 * and `base-uri` restrictions are safe to enforce globally.
 */
export function baselineHeaders(production: boolean) {
  return [
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
    ...(production
      ? [
          {
            key: "Strict-Transport-Security",
            value: "max-age=31536000; includeSubDomains",
          },
        ]
      : []),
  ];
}

/** Path of the OAuth consent screen of the MCP server (CF-173). */
export const CONSENT_PATH = "/oauth/consent";

/**
 * The consent screen decides who gets access to an account, so it is never
 * framed (not even by this origin: clickjacking), never cached, and its
 * `?request=` id is not sent on as a referrer.
 */
export const consentHeaders = [
  {
    key: "Content-Security-Policy",
    value: "frame-ancestors 'none'; base-uri 'self'; object-src 'none'",
  },
  { key: "X-Frame-Options", value: "DENY" },
  { key: "Cache-Control", value: "no-store" },
  { key: "Referrer-Policy", value: "no-referrer" },
];

/**
 * Desktop app sign-in pages (CF-234): `/desktop/login` carries the app's
 * state / PKCE challenge in its URL and hands out a one-time code, so it gets
 * the consent screen's rules (never framed, cached or sent as a referrer).
 */
export const DESKTOP_PATHS = "/desktop/:path*";

/** Later rules win for the same header, so the consent rule comes last. */
export function headerRules(production: boolean): HeaderRule[] {
  return [
    { source: "/:path*", headers: baselineHeaders(production) },
    { source: DESKTOP_PATHS, headers: consentHeaders },
    { source: CONSENT_PATH, headers: consentHeaders },
  ];
}
