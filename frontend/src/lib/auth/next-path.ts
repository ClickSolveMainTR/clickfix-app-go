/**
 * Returns `raw` when it is a safe same-origin path to continue to after
 * signing in (`/login?next=...`), otherwise `fallback`. Browsers treat a
 * backslash like "/", so a value starting "/<backslash>evil.com" would become
 * a protocol-relative redirect; control characters are rejected too.
 */
export function safeNextPath(raw: string | null | undefined, fallback: string) {
  if (
    !raw ||
    !raw.startsWith("/") ||
    raw.startsWith("//") ||
    /[\\\u0000-\u001f]/.test(raw)
  ) {
    return fallback;
  }
  return raw;
}

/** Reads `?next=` from the current URL (client only). */
export function nextFromLocation(fallback: string) {
  if (typeof window === "undefined") return fallback;
  return safeNextPath(
    new URLSearchParams(window.location.search).get("next"),
    fallback,
  );
}
