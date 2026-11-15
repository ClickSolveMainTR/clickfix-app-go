import type { NextRequest } from "next/server";

import { callbackGuardProviders, handlers } from "@/auth";
import { routes } from "@/config/routes";
import {
  isCrossSiteAppleCallback,
  resubmitSameSite,
} from "@/lib/server/apple-callback";
import {
  callbackErrorRedirect,
  callbackProviderId,
  rejectCallback,
} from "@/lib/server/auth-callback-guard";

/**
 * Callbacks that cannot succeed (no provider segment, a provider that is not
 * configured, no `code` / `state`) go to the sign-in page with an `error`
 * code instead of reaching Auth.js, which threw a TypeError for them
 * (CF-227, Sentry CLICKFIX-FRONT-END-8).
 */
async function guardCallback(request: NextRequest): Promise<Response | null> {
  const { pathname, searchParams } = request.nextUrl;
  if (callbackProviderId(pathname) === null) return null;
  let form: FormData | null = null;
  if (request.method === "POST") {
    try {
      form = await request.clone().formData();
    } catch {
      form = null; // not a form body; the guard sees no fields
    }
  }
  let providers;
  try {
    providers = await callbackGuardProviders();
  } catch {
    return null; // let Auth.js report a configuration problem itself
  }
  const error = rejectCallback(
    { method: request.method, pathname, query: searchParams, form },
    providers,
  );
  if (!error) return null;
  console.warn(
    `[auth][callback] rejected ${request.method} ${pathname}: ${error}`,
  );
  return callbackErrorRedirect(routes.guest.login, error);
}

export async function GET(request: NextRequest) {
  return (await guardCallback(request)) ?? handlers.GET(request);
}

/**
 * Apple's form_post callback is a cross-site POST, so the browser leaves the
 * SameSite=Lax session cookie off and "Link Apple" from the profile looked
 * like a new sign-in (OAuthNotLinked when registration is closed). That POST
 * is re-posted once from our own origin, where the session cookie is sent.
 */
export async function POST(request: NextRequest) {
  const rejected = await guardCallback(request);
  if (rejected) return rejected;
  if (isCrossSiteAppleCallback(request)) {
    return resubmitSameSite(await request.formData(), request.nextUrl.pathname);
  }
  return handlers.POST(request);
}
