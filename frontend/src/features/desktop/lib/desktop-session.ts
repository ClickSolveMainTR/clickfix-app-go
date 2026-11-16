/**
 * Request rules of `POST /desktop/session` (CF-234): the desktop app's web
 * view posts the app's access token here, the web server trades it for a
 * separate web view session and sets the Auth.js cookie. Free of imports
 * (`node --test`). The token travels only in the POST body or the
 * Authorization header, never in a URL, and is never logged.
 */

/** Largest accepted body (a JWT plus a short path). */
export const DESKTOP_SESSION_MAX_BODY = 8 * 1024;

const JWT = /^[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+$/;

/**
 * Only a same-origin POST may set the session: a cross-site form must not
 * sign the web view (or any browser) in to another account (login CSRF).
 * Unlike the BFF check, a request with neither `Sec-Fetch-Site` nor `Origin`
 * is refused too.
 */
export function isSameOriginPost(headers: {
  get(name: string): string | null;
}): boolean {
  const site = headers.get("sec-fetch-site");
  if (site) return site === "same-origin";
  const origin = headers.get("origin");
  if (!origin || origin === "null") return false;
  const host = headers.get("x-forwarded-host") ?? headers.get("host");
  if (!host) return false;
  try {
    return new URL(origin).host === host.split(",")[0]!.trim();
  } catch {
    return false;
  }
}

export type DesktopSessionInput = { accessToken: string; next: string | null };

/**
 * Reads the access token from `Authorization: Bearer`, a form body
 * (`access_token`, `next`) or a JSON body. Returns null when no well-formed
 * token is present.
 */
export function readDesktopSessionInput(
  authorization: string | null,
  contentType: string | null,
  body: string,
): DesktopSessionInput | null {
  let token = "";
  let next: string | null = null;
  const type = (contentType ?? "").split(";")[0]!.trim().toLowerCase();
  if (type === "application/x-www-form-urlencoded") {
    const form = new URLSearchParams(body);
    token = form.get("access_token") ?? "";
    next = form.get("next");
  } else if (type === "application/json" && body.trim()) {
    try {
      const parsed = JSON.parse(body) as Record<string, unknown>;
      if (typeof parsed.access_token === "string") token = parsed.access_token;
      if (typeof parsed.next === "string") next = parsed.next;
    } catch {
      return null;
    }
  }
  const bearer = /^Bearer\s+(.+)$/i.exec(authorization ?? "");
  if (!token && bearer) token = bearer[1]!.trim();
  token = token.trim();
  if (!JWT.test(token) || token.length > DESKTOP_SESSION_MAX_BODY) return null;
  return { accessToken: token, next };
}
