/**
 * Guard for Auth.js callback requests that cannot succeed (CF-227).
 *
 * Auth.js 5 reads `options.provider.type` for a POST to `/callback` before it
 * checks that a provider was resolved, so `POST /api/auth/callback` (no
 * provider segment) or a callback for a provider that is not configured
 * threw `TypeError: Cannot read properties of undefined (reading 'type')`
 * and was reported to Sentry. A callback without `code` (or without `state`
 * for a provider that checks it, such as Apple's form_post) fails deeper in
 * the OAuth check the same way. These requests are answered here with a
 * redirect to the sign-in page carrying an `error` code instead.
 *
 * Kept free of imports so the node:test suite can load it directly.
 */

export const AUTH_BASE_PATH = "/api/auth";

/** `error` codes the sign-in page shows for a rejected callback. */
export const CALLBACK_ERRORS = {
  /** No provider segment, or a provider that is not configured. */
  invalidProvider: "InvalidProvider",
  /** OAuth callback without the authorization `code` or the `state`. */
  incomplete: "OAuthCallbackError",
} as const;

export type CallbackError =
  (typeof CALLBACK_ERRORS)[keyof typeof CALLBACK_ERRORS];

/** The parts of a configured Auth.js provider the guard looks at. */
export type GuardProvider = {
  id: string;
  type: string;
  checks?: readonly string[];
};

type ProviderConfig = {
  id?: unknown;
  type?: unknown;
  checks?: unknown;
  options?: { id?: unknown; checks?: unknown } | null;
};

const stringList = (value: unknown): string[] | undefined =>
  Array.isArray(value)
    ? value.filter((v): v is string => typeof v === "string")
    : undefined;

/**
 * The id, type and checks Auth.js resolves for a configured provider: user
 * options (`Credentials({ id: "email-code" })`) win over the defaults.
 */
export function toGuardProvider(provider: unknown): GuardProvider | null {
  const resolved = (
    typeof provider === "function" ? provider() : provider
  ) as ProviderConfig | null;
  if (!resolved || typeof resolved !== "object") return null;
  const id = resolved.options?.id ?? resolved.id;
  if (typeof id !== "string" || typeof resolved.type !== "string") return null;
  const checks =
    stringList(resolved.options?.checks) ?? stringList(resolved.checks);
  return { id, type: resolved.type, ...(checks ? { checks } : {}) };
}

/**
 * "First Last" from Apple's `user` field, or `null` when it is missing or
 * incomplete. Apple sends that field (as JSON in the form_post) only on the
 * first sign-in, and it may carry no `name` at all.
 */
export function appleDisplayName(user: unknown): string | null {
  let value = user;
  if (typeof value === "string") {
    try {
      value = JSON.parse(value);
    } catch {
      return null;
    }
  }
  if (!value || typeof value !== "object") return null;
  const name = (value as { name?: unknown }).name;
  if (!name || typeof name !== "object") return null;
  const { firstName, lastName } = name as {
    firstName?: unknown;
    lastName?: unknown;
  };
  const full = [firstName, lastName]
    .filter((part): part is string => typeof part === "string")
    .map((part) => part.trim())
    .filter(Boolean)
    .join(" ");
  return full || null;
}

export type CallbackRequest = {
  method: string;
  pathname: string;
  query: URLSearchParams;
  /** Parsed form body of a POST, when it has one. */
  form?: { get(name: string): unknown } | null;
};

const field = (
  source: { get(name: string): unknown } | null | undefined,
  name: string,
) => {
  const value = source?.get(name);
  return typeof value === "string" ? value.trim() : "";
};

/**
 * The provider segment of a callback path, `""` when there is none, or `null`
 * when the path is not a callback (only callbacks are guarded).
 */
export function callbackProviderId(pathname: string): string | null {
  if (!pathname.startsWith(`${AUTH_BASE_PATH}/`)) return null;
  const parts = pathname
    .slice(AUTH_BASE_PATH.length)
    .split("/")
    .filter(Boolean);
  if (parts[0] !== "callback" || parts.length > 2) return null;
  const id = parts[1] ?? "";
  // Auth.js treats the literal "undefined" as a missing provider.
  return id === "undefined" ? "" : id;
}

/**
 * Returns the error code to redirect with when the callback cannot succeed,
 * or `null` to hand the request to Auth.js unchanged.
 */
export function rejectCallback(
  request: CallbackRequest,
  providers: readonly GuardProvider[],
): CallbackError | null {
  const id = callbackProviderId(request.pathname);
  if (id === null) return null;
  const provider = id ? providers.find((p) => p.id === id) : undefined;
  if (!provider) return CALLBACK_ERRORS.invalidProvider;

  if (provider.type !== "oauth" && provider.type !== "oidc") return null;

  // Apple answers with response_mode=form_post (fields in the POST body);
  // the other providers redirect back with a query string.
  const params = request.method === "POST" ? request.form : request.query;
  // The provider reported an error (e.g. the user cancelled): Auth.js maps
  // that to OAuthCallbackError itself.
  if (field(params, "error")) return null;
  if (!field(params, "code")) return CALLBACK_ERRORS.incomplete;
  if (provider.checks?.includes("state") && !field(params, "state")) {
    return CALLBACK_ERRORS.incomplete;
  }
  return null;
}

/**
 * Redirect to the sign-in page. The Location is relative so it never depends
 * on the proxy's host detection; 303 turns a form_post into a GET.
 */
export function callbackErrorRedirect(
  loginPath: string,
  error: CallbackError,
): Response {
  return new Response(null, {
    status: 303,
    headers: {
      location: `${loginPath}?${new URLSearchParams({ error })}`,
      "cache-control": "no-store",
    },
  });
}
