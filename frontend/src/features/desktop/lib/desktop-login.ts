/**
 * Decisions of the desktop app sign-in (CF-234, docs/design/masaustu.md §6),
 * kept apart from React and Next so they can be tested. Free of imports
 * (`node --test`).
 *
 * The desktop app opens `/desktop/login?state=...&challenge=...` in the
 * system browser. The page signs the user in the normal way, asks for
 * confirmation, gets a one-time code from the API and sends the browser to
 * `clickfix://auth/callback?code=...&state=...`.
 */

/** Same rules as the API (`internal/modules/desktoplogin`). */
const STATE = /^[A-Za-z0-9._~-]{16,128}$/;
const CHALLENGE = /^[A-Za-z0-9_-]{43}$/;
const CODE = /^[A-Za-z0-9_-]{43}$/;

export const DESKTOP_LOGIN_PATH = "/desktop/login";
export const DESKTOP_CALLBACK = "clickfix://auth/callback";

export type DesktopLoginParams = { state: string; challenge: string };

/** `state` and PKCE `challenge` from the query, or null when malformed. */
export function desktopLoginParams(
  state: string | null | undefined,
  challenge: string | null | undefined,
): DesktopLoginParams | null {
  const s = (state ?? "").trim();
  const c = (challenge ?? "").trim();
  return STATE.test(s) && CHALLENGE.test(c) ? { state: s, challenge: c } : null;
}

/** This page again, with the same parameters (sign-in comes back here). */
export function desktopLoginReturnPath(params: DesktopLoginParams) {
  const q = new URLSearchParams({
    state: params.state,
    challenge: params.challenge,
  });
  return `${DESKTOP_LOGIN_PATH}?${q.toString()}`;
}

/** Normal sign-in page that returns to this page afterwards (`?next=`). */
export function desktopLoginSignInPath(
  loginPath: string,
  params: DesktopLoginParams,
) {
  return `${loginPath}?next=${encodeURIComponent(desktopLoginReturnPath(params))}`;
}

export type DesktopLoginView =
  /** The link carries no valid state / challenge. */
  | "invalid"
  /** Session not known yet. */
  | "loading"
  /** Signed out: the page redirects to sign-in. */
  | "sign_in"
  /** Signed in: "ClickFix Masaüstü'ne giriş yapılıyor" confirmation. */
  | "confirm"
  /** The browser was sent to the app; "app did not open?" fallback. */
  | "sent"
  /** Code creation refused (impersonation, disabled account). */
  | "forbidden"
  | "failed";

export function desktopLoginView(s: {
  params: DesktopLoginParams | null;
  bootstrapped: boolean;
  authenticated: boolean;
  sent: boolean;
  /** HTTP status of a failed code request; 0 for network; null when none. */
  errorStatus: number | null;
}): DesktopLoginView {
  if (!s.params) return "invalid";
  if (!s.bootstrapped) return "loading";
  if (!s.authenticated) return "sign_in";
  if (s.errorStatus !== null)
    return s.errorStatus === 403 ? "forbidden" : "failed";
  return s.sent ? "sent" : "confirm";
}

/**
 * The API's redirect URL when it is exactly the app callback for this state
 * with a well-formed code; anything else is never followed.
 */
export function safeDesktopCallback(
  raw: string | null | undefined,
  state: string,
): string | null {
  if (!raw || !raw.startsWith(`${DESKTOP_CALLBACK}?`)) return null;
  const q = new URLSearchParams(raw.slice(DESKTOP_CALLBACK.length + 1));
  const code = q.get("code") ?? "";
  if (!CODE.test(code) || q.get("state") !== state) return null;
  const keys = [...q.keys()];
  if (keys.length !== 2) return null;
  return raw;
}

/** Desktop platform label for the device list. */
export function desktopOSLabel(os: string | null | undefined) {
  switch (os) {
    case "macos":
      return "macOS";
    case "windows":
      return "Windows";
    case "linux":
      return "Linux";
    default:
      return null;
  }
}
