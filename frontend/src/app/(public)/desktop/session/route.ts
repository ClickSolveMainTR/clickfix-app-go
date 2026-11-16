import { NextResponse } from "next/server";

import { routes } from "@/config/routes";
import {
  DESKTOP_SESSION_MAX_BODY,
  isSameOriginPost,
  readDesktopSessionInput,
} from "@/features/desktop/lib/desktop-session";
import { safeNextPath } from "@/lib/auth/next-path";
import { persistApiTokens } from "@/lib/server/auth-tokens";
import { clientIpFromHeaders, fetchUpstream } from "@/lib/server/upstream";

/**
 * Desktop app web view session (CF-234, docs/design/masaustu.md §6).
 *
 * The desktop app's web view posts the app's access token (form field or
 * JSON `access_token`, or `Authorization: Bearer`) with an optional `next`
 * path. The web server asks the API for a separate web view session
 * (`POST /v1/auth/desktop/web-session`; the app's own refresh token stays in
 * the OS keychain), stores it in the Auth.js session cookie and redirects
 * (303) to `next` or the panel home. The token never appears in a URL and is
 * never logged; only same-origin posts are accepted (login CSRF).
 */
export const dynamic = "force-dynamic";

function jsonError(status: number, code: string, message: string) {
  return NextResponse.json(
    { success: false, error: { code, message } },
    { status, headers: { "Cache-Control": "no-store" } },
  );
}

type WebSessionEnvelope = {
  data?: {
    access_token?: string;
    refresh_token?: string;
    expires_in?: number;
    user?: { uuid?: string; email?: string };
  };
  error?: { code?: string; message?: string };
};

export async function POST(request: Request) {
  if (!isSameOriginPost(request.headers)) {
    return jsonError(
      403,
      "CROSS_SITE_REQUEST_BLOCKED",
      "Cross-site requests are not allowed",
    );
  }
  const length = Number(request.headers.get("content-length") ?? "0");
  if (length > DESKTOP_SESSION_MAX_BODY) {
    return jsonError(413, "PAYLOAD_TOO_LARGE", "Request body is too large");
  }
  const text = (await request.text()).slice(0, DESKTOP_SESSION_MAX_BODY + 1);
  if (text.length > DESKTOP_SESSION_MAX_BODY) {
    return jsonError(413, "PAYLOAD_TOO_LARGE", "Request body is too large");
  }
  const input = readDesktopSessionInput(
    request.headers.get("authorization"),
    request.headers.get("content-type"),
    text,
  );
  if (!input) {
    return jsonError(400, "VALIDATION_ERROR", "Access token is required");
  }

  const clientIp = clientIpFromHeaders(request.headers);
  const userAgent = request.headers.get("user-agent");
  let status: number;
  let envelope: WebSessionEnvelope | null = null;
  try {
    const result = await fetchUpstream("auth/desktop/web-session", {
      method: "POST",
      headers: {
        Accept: "application/json",
        Authorization: `Bearer ${input.accessToken}`,
        ...(userAgent ? { "User-Agent": userAgent } : {}),
        ...(clientIp ? { "X-Forwarded-For": clientIp } : {}),
      },
    });
    status = result.status;
    try {
      envelope = JSON.parse(
        new TextDecoder().decode(result.body),
      ) as WebSessionEnvelope;
    } catch {
      envelope = null;
    }
  } catch {
    // No token or body in the log line.
    console.error("[desktop/session] upstream unavailable");
    return jsonError(503, "UPSTREAM_UNAVAILABLE", "Service unavailable");
  }

  const data = envelope?.data;
  if (
    status >= 400 ||
    !data?.access_token ||
    !data.refresh_token ||
    !data.user?.uuid
  ) {
    const code = envelope?.error?.code ?? "DESKTOP_SESSION_FAILED";
    return jsonError(
      status >= 400 ? status : 502,
      code,
      envelope?.error?.message ?? "Desktop session could not be created",
    );
  }

  await persistApiTokens({
    accessToken: data.access_token,
    refreshToken: data.refresh_token,
    userId: data.user.uuid,
    email: data.user.email ?? null,
    impersonatorUuid: null,
    expiresIn: data.expires_in,
  });

  // Relative Location: behind the proxy `request.url` may carry the
  // internal host, never the public one.
  const target = safeNextPath(input.next, routes.cms.home);
  return new NextResponse(null, {
    status: 303,
    headers: { Location: target, "Cache-Control": "no-store" },
  });
}
