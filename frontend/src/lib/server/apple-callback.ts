const APPLE_ORIGIN = "https://appleid.apple.com";

/**
 * True for Apple's `response_mode=form_post` callback arriving from
 * appleid.apple.com. `Sec-Fetch-Site` decides when the browser sends it; the
 * Origin header covers browsers that do not. The re-posted request comes from
 * our own origin, so it never matches again.
 */
export function isCrossSiteAppleCallback(request: {
  method: string;
  headers: Headers;
  nextUrl: { pathname: string };
}): boolean {
  if (request.method !== "POST") return false;
  if (!request.nextUrl.pathname.endsWith("/callback/apple")) return false;
  const site = request.headers.get("sec-fetch-site");
  if (site) return site === "cross-site";
  return request.headers.get("origin") === APPLE_ORIGIN;
}

const escapeHtml = (value: string) =>
  value
    .replaceAll("&", "&amp;")
    .replaceAll('"', "&quot;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll("'", "&#39;");

/**
 * A page that posts the same fields back to `action` from our origin: a
 * same-site POST carries the Lax session cookie, so Auth.js sees the signed-in
 * member and links Apple instead of treating it as a new sign-in.
 */
export function resubmitSameSite(form: FormData, action: string): Response {
  const inputs = [...form.entries()]
    .filter((entry): entry is [string, string] => typeof entry[1] === "string")
    .map(
      ([name, value]) =>
        `<input type="hidden" name="${escapeHtml(name)}" value="${escapeHtml(value)}">`,
    )
    .join("");
  const html =
    `<!doctype html><html><head><meta charset="utf-8"><meta name="robots" content="noindex"><title>ClickFix</title></head>` +
    `<body><form id="f" method="post" action="${escapeHtml(action)}">${inputs}` +
    `<noscript><button type="submit">Continue</button></noscript></form>` +
    `<script>document.getElementById("f").submit()</script></body></html>`;
  return new Response(html, {
    status: 200,
    headers: {
      "content-type": "text/html; charset=utf-8",
      "cache-control": "no-store",
      "referrer-policy": "no-referrer",
    },
  });
}
