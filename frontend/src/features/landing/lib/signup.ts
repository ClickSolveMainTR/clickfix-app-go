import { upstreamConfig } from "@/config/api";

/**
 * Platform sign-up mode (CF-143, `auth_settings.signup_mode`): `open` lets
 * anyone open an account and an organization at `/register`; `invite` and
 * `closed` admit new accounts only through invitations.
 */
export type SignupMode = "open" | "invite" | "closed";

/**
 * Sign-up mode from the public app config (cached 5 minutes unless
 * `fresh`, as on `/register`). Answers `invite` when the API cannot be
 * reached, so the site asks for a demo instead of showing a form that
 * cannot work.
 */
export async function fetchSignupMode({
  fresh = false,
}: { fresh?: boolean } = {}): Promise<SignupMode> {
  try {
    const response = await fetch(`${upstreamConfig.baseUrl}/app/config`, {
      headers: { Accept: "application/json" },
      ...(fresh ? { cache: "no-store" } : { next: { revalidate: 300 } }),
      signal: AbortSignal.timeout(3000),
    });
    if (!response.ok) return "invite";
    const json = (await response.json()) as {
      data?: { signup_mode?: string; self_serve_enabled?: boolean };
    };
    const mode = json.data?.signup_mode;
    if (mode === "open" || mode === "invite" || mode === "closed") return mode;
    return json.data?.self_serve_enabled ? "open" : "invite";
  } catch {
    return "invite";
  }
}

/**
 * Whether businesses can open an account themselves (`self_serve_enabled`,
 * i.e. sign-up mode `open`). The landing CTAs send visitors to `/register`
 * while it is on and ask for a demo otherwise.
 */
export async function isSelfSignupOpen(): Promise<boolean> {
  return (await fetchSignupMode()) === "open";
}
