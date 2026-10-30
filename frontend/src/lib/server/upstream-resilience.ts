/**
 * Dependency-free helpers shared by the BFF route handlers (CF-153). Kept
 * free of `@/` imports so `upstream-resilience.test.mjs` can load it with
 * esbuild alone.
 */

/**
 * Statuses the Fetch spec forbids a body on: `new Response(body, { status })`
 * throws "Invalid response status code" for them even when the body is an
 * empty buffer (CLICKFIX-FRONT-END-5).
 */
export const NULL_BODY_STATUSES: ReadonlySet<number> = new Set([204, 205, 304]);

/** Builds a proxied response, dropping the body where the status forbids one. */
export function proxyResponse(
  body: BodyInit | null,
  init: ResponseInit & { status: number },
): Response {
  return new Response(NULL_BODY_STATUSES.has(init.status) ? null : body, init);
}

/** Error code the BFF answers with when the Go API cannot be reached. */
export const UPSTREAM_UNAVAILABLE = "UPSTREAM_UNAVAILABLE";

/**
 * The Go API could not be reached (connection refused or reset, DNS, socket
 * closed mid-body), typically while it restarts during a deploy. Expected
 * and transient: route handlers answer 503 and log a warning instead of
 * reporting an exception.
 */
export class UpstreamUnavailableError extends Error {
  constructor(cause: unknown) {
    super("upstream unavailable", { cause });
    this.name = "UpstreamUnavailableError";
  }
}

const NETWORK_CODES = new Set([
  "ECONNREFUSED",
  "ECONNRESET",
  "EPIPE",
  "ENOTFOUND",
  "EAI_AGAIN",
  "ETIMEDOUT",
  "EHOSTUNREACH",
  "ENETUNREACH",
  "UND_ERR_SOCKET",
  "UND_ERR_CLOSED",
  "UND_ERR_CONNECT_TIMEOUT",
]);

function errorCodes(err: unknown, depth = 0): string[] {
  if (!err || typeof err !== "object" || depth > 4) return [];
  const out: string[] = [];
  const code = (err as { code?: unknown }).code;
  if (typeof code === "string") out.push(code);
  const nested = (err as { errors?: unknown }).errors;
  if (Array.isArray(nested)) {
    for (const e of nested) out.push(...errorCodes(e, depth + 1));
  }
  out.push(...errorCodes((err as { cause?: unknown }).cause, depth + 1));
  return out;
}

function isAbort(err: unknown): boolean {
  const name = (err as { name?: unknown } | null)?.name;
  return name === "AbortError" || name === "TimeoutError";
}

/**
 * True for fetch failures that mean "the API is not answering" rather than a
 * bug: undici's `TypeError: fetch failed` / `terminated` or a known socket
 * error code anywhere in the cause chain. Aborts are not.
 */
export function isUpstreamNetworkError(err: unknown): boolean {
  if (err instanceof UpstreamUnavailableError) return true;
  if (isAbort(err)) return false;
  if (errorCodes(err).some((code) => NETWORK_CODES.has(code))) return true;
  return err instanceof TypeError;
}

/** The request never reached the API, so even a POST is safe to resend. */
function connectionRefused(err: unknown): boolean {
  return errorCodes(err).includes("ECONNREFUSED");
}

const IDEMPOTENT = new Set(["GET", "HEAD", "OPTIONS"]);

export type RetryOptions = {
  method: string;
  /** Waits between attempts; its length is the number of retries. */
  delaysMs?: readonly number[];
  sleep?: (ms: number) => Promise<void>;
};

const DEFAULT_DELAYS_MS = [250, 750] as const;

const defaultSleep = (ms: number) =>
  new Promise<void>((resolve) => setTimeout(resolve, ms));

/**
 * Runs `attempt` and retries it briefly when the API is unreachable: any
 * network error for idempotent methods, only "connection refused" for the
 * rest. Throws UpstreamUnavailableError once retries are spent; other
 * errors (aborts, bugs) are rethrown untouched.
 */
export async function withUpstreamRetry<T>(
  attempt: () => Promise<T>,
  { method, delaysMs = DEFAULT_DELAYS_MS, sleep = defaultSleep }: RetryOptions,
): Promise<T> {
  const idempotent = IDEMPOTENT.has(method.toUpperCase());
  for (let i = 0; ; i++) {
    try {
      return await attempt();
    } catch (err) {
      if (!isUpstreamNetworkError(err)) throw err;
      const retryable = idempotent || connectionRefused(err);
      if (!retryable || i >= delaysMs.length) {
        throw err instanceof UpstreamUnavailableError
          ? err
          : new UpstreamUnavailableError(err);
      }
      await sleep(delaysMs[i]!);
    }
  }
}

/** Envelope answered when the API is unreachable (API_CONVENTIONS.md shape). */
export function upstreamUnavailableResponse(headers?: HeadersInit): Response {
  const out = new Headers(headers);
  out.set("Content-Type", "application/json");
  out.set("Cache-Control", "no-store");
  out.set("Retry-After", "5");
  return new Response(
    JSON.stringify({
      success: false,
      error: {
        code: UPSTREAM_UNAVAILABLE,
        message: "The service is temporarily unavailable. Try again shortly.",
      },
    }),
    { status: 503, headers: out },
  );
}
