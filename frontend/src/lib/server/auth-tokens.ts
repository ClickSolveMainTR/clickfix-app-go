import { decode, encode } from "next-auth/jwt";
import { cookies } from "next/headers";

const SESSION_COOKIE_BASES = [
  "authjs.session-token",
  "__Secure-authjs.session-token",
] as const;

const SESSION_COOKIE_CHUNKS = 12;

/**
 * Auth.js uses the `__Secure-` cookie only when the app is served over HTTPS. A production build
 * served over plain HTTP (E2E stack on 127.0.0.1, AUTH_URL=http://...) gets the plain cookie, so
 * follow AUTH_URL here too.
 */
function secureCookies() {
  if (process.env.NODE_ENV !== "production") return false;
  return !(process.env.AUTH_URL ?? "").startsWith("http://");
}

export function sessionCookieName() {
  return secureCookies()
    ? "__Secure-authjs.session-token"
    : "authjs.session-token";
}

function sessionCookieOptions() {
  return {
    httpOnly: true,
    secure: secureCookies(),
    sameSite: "lax" as const,
    path: "/",
  };
}

/** Expire the Auth.js session cookie and any chunked variants Auth.js may create. */
async function expireSessionCookies() {
  const jar = await cookies();
  const opts = { ...sessionCookieOptions(), maxAge: 0 };
  for (const base of SESSION_COOKIE_BASES) {
    jar.set(base, "", opts);
    for (let i = 0; i < SESSION_COOKIE_CHUNKS; i += 1) {
      jar.set(`${base}.${i}`, "", opts);
    }
  }
}

async function readSessionToken() {
  const jar = await cookies();
  const name = sessionCookieName();
  const raw = jar.get(name)?.value;
  if (!raw || !process.env.AUTH_SECRET) {
    // Auth.js may have split a large JWT across numbered chunk cookies.
    const chunks: string[] = [];
    for (let i = 0; i < SESSION_COOKIE_CHUNKS; i += 1) {
      const part = jar.get(`${name}.${i}`)?.value;
      if (!part) break;
      chunks.push(part);
    }
    if (chunks.length === 0) return null;
    return decode({
      token: chunks.join(""),
      secret: process.env.AUTH_SECRET!,
      salt: name,
    });
  }
  return decode({
    token: raw,
    secret: process.env.AUTH_SECRET,
    salt: name,
  });
}

export async function getApiTokens() {
  const token = await readSessionToken();
  if (!token) {
    return {
      accessToken: null as string | null,
      refreshToken: null as string | null,
      userId: null as string | null,
      email: null as string | null,
      impersonatorUuid: null as string | null,
    };
  }
  return {
    accessToken: (token.accessToken as string | undefined) ?? null,
    refreshToken: (token.refreshToken as string | undefined) ?? null,
    userId: (token.sub as string | undefined) ?? null,
    email: (token.email as string | undefined) ?? null,
    impersonatorUuid: (token.impersonatorUuid as string | undefined) ?? null,
  };
}

export async function persistApiTokens(input: {
  accessToken: string;
  refreshToken: string;
  userId: string;
  email?: string | null;
  impersonatorUuid?: string | null;
  expiresIn?: number;
}) {
  if (!process.env.AUTH_SECRET) {
    throw new Error("AUTH_SECRET is not configured");
  }

  const existing = await readSessionToken();
  const maxAge = input.expiresIn ?? 15 * 60;
  const name = sessionCookieName();
  // Organization SSO grants belong to the signed-in user only: an
  // impersonation switch (or another account) starts without them.
  const sameUser = existing?.sub === input.userId;
  const payload = await encode({
    token: {
      ...existing,
      orgSso: sameUser ? existing?.orgSso : undefined,
      sub: input.userId,
      email: input.email ?? existing?.email,
      accessToken: input.accessToken,
      refreshToken: input.refreshToken,
      impersonatorUuid: input.impersonatorUuid ?? undefined,
    },
    secret: process.env.AUTH_SECRET,
    salt: name,
    maxAge,
  });

  // Drop any leftover Auth.js chunks before writing a single cookie.
  await expireSessionCookies();

  const jar = await cookies();
  jar.set(name, payload, {
    ...sessionCookieOptions(),
    maxAge,
  });
}

/**
 * Organization SSO grant (CF-128): the API's tenant-only access token from an
 * organization IdP sign-in. It has no refresh and only works on
 * `/v1/tenant/*` of that organization, so it rides next to the global pair
 * and the BFF uses it for that organization's tenant requests.
 */
export type OrgSSOGrant = {
  uuid: string;
  slug: string;
  token: string;
  /** Unix seconds. */
  exp: number;
};

function liveGrants(raw: unknown): OrgSSOGrant[] {
  if (!Array.isArray(raw)) return [];
  const now = Math.floor(Date.now() / 1000);
  return (raw as OrgSSOGrant[]).filter(
    (g) => g && typeof g.token === "string" && g.exp > now + 5,
  );
}

/** Live grant for an `X-Organization` reference (slug or uuid). */
export async function getOrgSSOGrant(
  organization: string,
): Promise<OrgSSOGrant | null> {
  const token = await readSessionToken();
  const ref = organization.trim().toLowerCase();
  return (
    liveGrants(token?.orgSso).find(
      (g) => g.uuid.toLowerCase() === ref || g.slug.toLowerCase() === ref,
    ) ?? null
  );
}

/** Stores a grant on the current session (one per organization). */
export async function persistOrgSSOGrant(grant: OrgSSOGrant) {
  if (!process.env.AUTH_SECRET) {
    throw new Error("AUTH_SECRET is not configured");
  }
  const existing = await readSessionToken();
  if (!existing?.sub) return false;
  const name = sessionCookieName();
  const grants = [
    ...liveGrants(existing.orgSso).filter((g) => g.uuid !== grant.uuid),
    grant,
  ].slice(-5);
  // The session cookie keeps the lifetime Auth.js gave it.
  const exp = typeof existing.exp === "number" ? existing.exp : undefined;
  const maxAge = exp
    ? Math.max(exp - Math.floor(Date.now() / 1000), 60)
    : 15 * 60;
  const payload = await encode({
    token: { ...existing, orgSso: grants },
    secret: process.env.AUTH_SECRET,
    salt: name,
    maxAge,
  });
  await expireSessionCookies();
  const jar = await cookies();
  jar.set(name, payload, { ...sessionCookieOptions(), maxAge });
  return true;
}

export async function clearAuthSessionCookie() {
  await expireSessionCookies();
}
