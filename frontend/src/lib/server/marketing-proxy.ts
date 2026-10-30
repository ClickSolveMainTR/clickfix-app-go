import { revalidateTag } from "next/cache";

import { MARKETING_CACHE_TAG } from "@/features/landing/lib/marketing";
import {
  CONVERSION_COOKIE,
  FIRST_TOUCH_COOKIE,
  LAST_TOUCH_COOKIE,
} from "@/features/landing/lib/tracking";

/** Header the API reads the sign-up attribution from (marketing module). */
const ATTRIBUTION_HEADER = "X-ClickFix-Attribution";

/** Password sign-up and self-serve organization creation (CF-144). */
export function isSignupPath(path: string, method: string) {
  return (
    method.toUpperCase() === "POST" &&
    (path === "auth/register" || path === "onboarding/organizations")
  );
}

function cookieValue(header: string | null, name: string): string | null {
  if (!header) return null;
  for (const part of header.split(";")) {
    const [key, ...rest] = part.trim().split("=");
    if (key === name) return rest.join("=");
  }
  return null;
}

function touch(raw: string | null): unknown {
  if (!raw || raw.length > 2000) return undefined;
  try {
    const value: unknown = JSON.parse(decodeURIComponent(raw));
    return value && typeof value === "object" ? value : undefined;
  } catch {
    return undefined;
  }
}

/**
 * Forwards the first / last touch cookies (written by the marketing site
 * after analytics consent) to the API on sign-up; the API validates and
 * stores them. Nothing is sent without them.
 */
export function forwardAttribution(request: Request, headers: Headers) {
  const cookies = request.headers.get("cookie");
  const ft = touch(cookieValue(cookies, FIRST_TOUCH_COOKIE));
  const lt = touch(cookieValue(cookies, LAST_TOUCH_COOKIE));
  if (!ft && !lt) return;
  const value = encodeURIComponent(JSON.stringify({ ft, lt }));
  if (value.length <= 4096) headers.set(ATTRIBUTION_HEADER, value);
}

/**
 * After a successful sign-up, marks the browser so the next page fires the
 * conversion events once (`SignupConversion`, consent permitting).
 */
export function markConversion(response: Response) {
  if (response.status !== 201) return response;
  const secure = process.env.NODE_ENV === "production" ? "; Secure" : "";
  response.headers.append(
    "Set-Cookie",
    `${CONVERSION_COOKIE}=1; Path=/; Max-Age=3600; SameSite=Lax${secure}`,
  );
  return response;
}

/** Saved tracking settings reach the marketing pages without a redeploy. */
export function revalidateMarketing(
  path: string,
  method: string,
  status: number,
) {
  if (
    status < 400 &&
    method.toUpperCase() !== "GET" &&
    path.startsWith("platform/marketing")
  ) {
    revalidateTag(MARKETING_CACHE_TAG, { expire: 0 });
  }
}
