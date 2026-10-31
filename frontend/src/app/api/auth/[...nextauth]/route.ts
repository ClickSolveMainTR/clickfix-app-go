import type { NextRequest } from "next/server";

import { handlers } from "@/auth";
import {
  isCrossSiteAppleCallback,
  resubmitSameSite,
} from "@/lib/server/apple-callback";

export const { GET } = handlers;

/**
 * Apple's form_post callback is a cross-site POST, so the browser leaves the
 * SameSite=Lax session cookie off and "Link Apple" from the profile looked
 * like a new sign-in (OAuthNotLinked when registration is closed). That POST
 * is re-posted once from our own origin, where the session cookie is sent.
 */
export async function POST(request: NextRequest) {
  if (isCrossSiteAppleCallback(request)) {
    return resubmitSameSite(await request.formData(), request.nextUrl.pathname);
  }
  return handlers.POST(request);
}
