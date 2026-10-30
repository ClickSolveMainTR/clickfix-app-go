import { decode, encode } from "next-auth/jwt";

import { upstreamConfig } from "@/config/api";
import { routes } from "@/config/routes";
import { safeNextPath } from "@/lib/auth/next-path";
import { getApiTokens, persistOrgSSOGrant } from "@/lib/server/auth-tokens";
import { clientIpFromHeaders } from "@/lib/server/upstream";

/**
 * Organization SSO (CF-128) through the BFF. The generic proxy never forwards
 * browser cookies and blocks cross-site POSTs, but SP-initiated SSO needs the
 * API's `__Host-clickfix-sso` state cookie to round-trip and the IdP posts
 * the SAML response cross-site (the state cookie and the signed assertion
 * protect that POST). Only this cookie crosses the proxy.
 *
 * The callback's token is tenant-only with no refresh: it cannot replace the
 * panel session, so the BFF keeps it as an organization grant on the signed-in
 * user's session (same user only) and the proxy sends it on that
 * organization's tenant requests. The browser never sees it.
 *
 * The SAML POST is cross-site, so the Lax session cookie is not on it: every
 * callback seals the token into a short-lived cookie and 303s to `finish`, a
 * top-level GET that carries the session.
 */
const STATE_COOKIE = "__Host-clickfix-sso";
/** Where to land after the callback (`?next=` of `start`), BFF-only. */
const NEXT_COOKIE = "clickfix-sso-next";
/** Sealed callback token on its way to `finish` (60 s). */
const PENDING_COOKIE = "clickfix-sso-pending";
const COOKIE_PATH = "/api/v1/public/sso";
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

/** Result flag the landing page reads (`?sso=`). */
type Outcome = "ok" | "failed" | "mismatch" | "session";

type RouteContext = {
  params: Promise<{ organization: string; step: string[] }>;
};

function upstreamUrl(organization: string, step: string, search = "") {
  return `${upstreamConfig.baseUrl.replace(/\/$/, "")}/public/sso/${organization}/${step}${search}`;
}

function readCookie(request: Request, name: string): string | null {
  for (const part of (request.headers.get("cookie") ?? "").split(";")) {
    const [key, ...value] = part.trim().split("=");
    if (key === name) return value.join("=");
  }
  return null;
}

function upstreamHeaders(request: Request): Headers {
  const headers = new Headers({ Accept: "application/json" });
  const ip = clientIpFromHeaders(request.headers);
  if (ip) headers.set("X-Forwarded-For", ip);
  const state = readCookie(request, STATE_COOKIE);
  if (state) headers.set("Cookie", `${STATE_COOKIE}=${state}`);
  return headers;
}

/** Copies only the SSO state cookie from the API response. */
function withStateCookie(from: Response, to: Response): Response {
  for (const cookie of from.headers.getSetCookie()) {
    if (cookie.startsWith(`${STATE_COOKIE}=`)) {
      to.headers.append("Set-Cookie", cookie);
    }
  }
  to.headers.set("Cache-Control", "no-store");
  return to;
}

function cookie(
  name: string,
  value: string,
  maxAge: number,
  sameSite: "Lax" | "None",
) {
  const secure = process.env.NODE_ENV === "production";
  return [
    `${name}=${encodeURIComponent(value)}`,
    `Path=${COOKIE_PATH}`,
    `Max-Age=${maxAge}`,
    "HttpOnly",
    // None needs Secure; the API's state cookie already needs HTTPS.
    secure ? `Secure; SameSite=${sameSite}` : "SameSite=Lax",
  ].join("; ");
}

/** `next` is read on the cross-site SAML POST, so it is SameSite=None. */
const nextCookie = (value: string, maxAge: number) =>
  cookie(NEXT_COOKIE, value, maxAge, "None");

type Pending = { token: string; expiresIn: number };

async function seal(organization: string, value: Pending) {
  return encode({
    token: { ...value, org: organization },
    secret: process.env.AUTH_SECRET ?? "",
    salt: PENDING_COOKIE,
    maxAge: 60,
  });
}

async function unseal(organization: string, raw: string | null) {
  if (!raw || !process.env.AUTH_SECRET) return null;
  try {
    const v = await decode({
      token: decodeURIComponent(raw),
      secret: process.env.AUTH_SECRET,
      salt: PENDING_COOKIE,
    });
    if (v?.org !== organization) return null;
    return typeof v.token === "string" && typeof v.expiresIn === "number"
      ? { token: v.token, expiresIn: v.expiresIn }
      : null;
  } catch {
    return null;
  }
}

/** Reads the access token's subject (user uuid); the API just issued it. */
function tokenSubject(token: string): string | null {
  try {
    const part = token.split(".")[1] ?? "";
    const json = Buffer.from(part, "base64url").toString("utf8");
    const sub = (JSON.parse(json) as { sub?: unknown }).sub;
    return typeof sub === "string" ? sub : null;
  } catch {
    return null;
  }
}

/** Slug of the organization, read with the new token itself. */
async function organizationSlug(organization: string, token: string) {
  const res = await fetch(
    `${upstreamConfig.baseUrl.replace(/\/$/, "")}/tenant/context`,
    {
      headers: {
        Accept: "application/json",
        Authorization: `Bearer ${token}`,
        "X-Organization": organization,
      },
      cache: "no-store",
    },
  );
  if (!res.ok) return null;
  const payload = (await res.json().catch(() => null)) as {
    data?: { organization?: { slug?: string } };
  } | null;
  return payload?.data?.organization?.slug ?? null;
}

/** Turns the sealed callback token into an organization grant. */
async function grant(
  organization: string,
  pending: Pending | null,
): Promise<Outcome> {
  if (!pending || pending.expiresIn <= 0) return "failed";
  const { userId } = await getApiTokens();
  if (!userId) return "session";
  // The IdP account must be the panel user, never a different one.
  if (tokenSubject(pending.token)?.toLowerCase() !== userId.toLowerCase()) {
    return "mismatch";
  }
  const slug = await organizationSlug(organization, pending.token);
  if (!slug) return "failed";
  const saved = await persistOrgSSOGrant({
    uuid: organization.toLowerCase(),
    slug,
    token: pending.token,
    exp: Math.floor(Date.now() / 1000) + pending.expiresIn,
  });
  return saved ? "ok" : "session";
}

function redirect(location: string) {
  // 303 turns the IdP's POST into a GET.
  return new Response(null, {
    status: 303,
    headers: { Location: location, "Cache-Control": "no-store" },
  });
}

async function callback(request: Request, organization: string, step: string) {
  const url = new URL(request.url);
  const headers = upstreamHeaders(request);
  let body: string | undefined;
  if (request.method === "POST") {
    headers.set(
      "Content-Type",
      request.headers.get("content-type") ??
        "application/x-www-form-urlencoded",
    );
    body = await request.text();
  }
  const res = await fetch(upstreamUrl(organization, step, url.search), {
    method: request.method,
    headers,
    body,
    cache: "no-store",
    redirect: "manual",
  });
  const payload = res.ok
    ? ((await res.json().catch(() => null)) as {
        data?: { access_token?: string; expires_in?: number };
      } | null)
    : null;
  const token = payload?.data?.access_token;
  const out = redirect(`${COOKIE_PATH}/${organization}/finish`);
  if (token && process.env.AUTH_SECRET) {
    const sealed = await seal(organization, {
      token,
      expiresIn: payload?.data?.expires_in ?? 0,
    });
    out.headers.append("Set-Cookie", cookie(PENDING_COOKIE, sealed, 60, "Lax"));
  }
  return withStateCookie(res, out);
}

async function finish(request: Request, organization: string) {
  const url = new URL(request.url);
  const pending = await unseal(
    organization,
    readCookie(request, PENDING_COOKIE),
  );
  const outcome = await grant(organization, pending);
  const saved = readCookie(request, NEXT_COOKIE);
  const next = safeNextPath(
    saved ? decodeURIComponent(saved) : null,
    routes.cms.settings.security,
  );
  // Without a panel session (or with another account) the SSO entry page
  // explains it and starts over after sign-in.
  const entry = outcome === "session" || outcome === "mismatch";
  const target = new URL(entry ? routes.guest.loginSSO : next, url.origin);
  if (entry) {
    target.searchParams.set("organization", organization);
    target.searchParams.set("next", next);
  }
  target.searchParams.set("sso", outcome);
  const out = redirect(target.pathname + target.search);
  out.headers.append("Set-Cookie", nextCookie("", 0));
  out.headers.append("Set-Cookie", cookie(PENDING_COOKIE, "", 0, "Lax"));
  return out;
}

async function start(request: Request, organization: string) {
  const res = await fetch(upstreamUrl(organization, "start"), {
    headers: upstreamHeaders(request),
    cache: "no-store",
  });
  const out = new Response(await res.arrayBuffer(), {
    status: res.status,
    headers: {
      "Content-Type": res.headers.get("content-type") ?? "application/json",
    },
  });
  const next = new URL(request.url).searchParams.get("next");
  if (res.ok && next) {
    out.headers.append(
      "Set-Cookie",
      nextCookie(safeNextPath(next, routes.cms.home), 300),
    );
  }
  return withStateCookie(res, out);
}

async function handle(request: Request, context: RouteContext) {
  const { organization, step } = await context.params;
  const path = step.join("/");
  if (!UUID.test(organization)) {
    return Response.json(
      { success: false, error: "not_found" },
      { status: 404 },
    );
  }
  if (path === "oidc/callback" && request.method === "GET") {
    return callback(request, organization, path);
  }
  if (path === "saml/callback" && request.method === "POST") {
    return callback(request, organization, path);
  }
  if (path === "finish" && request.method === "GET") {
    return finish(request, organization);
  }
  if (path === "start" && request.method === "GET") {
    return start(request, organization);
  }
  if (path === "saml/metadata" && request.method === "GET") {
    const res = await fetch(upstreamUrl(organization, path), {
      headers: upstreamHeaders(request),
      cache: "no-store",
    });
    return new Response(await res.arrayBuffer(), {
      status: res.status,
      headers: {
        "Content-Type":
          res.headers.get("content-type") ?? "application/octet-stream",
        "Cache-Control": "no-store",
      },
    });
  }
  return Response.json({ success: false, error: "not_found" }, { status: 404 });
}

export const GET = handle;
export const POST = handle;
