/**
 * Decisions of the MCP consent screen (CF-173), kept apart from the React
 * page so they can be tested: what the screen shows in each situation and
 * what exactly is sent to the API. Free of imports (`node --test`).
 */

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

/** The `request` query value when it is a UUID; anything else is not a consent request. */
export function consentRequestId(
  raw: string | null | undefined,
): string | null {
  const value = (raw ?? "").trim();
  return UUID.test(value) ? value.toLowerCase() : null;
}

/** Where a signed-out visitor signs in, coming back to the same request. */
export function consentLoginPath(loginPath: string, requestId: string) {
  const next = `/oauth/consent?request=${encodeURIComponent(requestId)}`;
  return `${loginPath}?next=${encodeURIComponent(next)}`;
}

export type ConsentOrganization = {
  uuid: string;
  name: string;
  tool_count: number | null;
  can_write: boolean;
};

export type ConsentView =
  /** The link carries no valid request id. */
  | "invalid"
  /** Session not known yet, or the request is loading. */
  | "loading"
  /** Signed out: the page redirects to sign-in. */
  | "sign_in"
  /** The request expired, was answered or belongs to someone else (404). */
  | "expired"
  /** The member may not give this consent (403). */
  | "forbidden"
  | "failed"
  /** Signed in, but no organization where the member may connect. */
  | "no_organization"
  | "ready";

export function consentView(s: {
  requestId: string | null;
  bootstrapped: boolean;
  authenticated: boolean;
  loading: boolean;
  /** HTTP status of a failed load; 0 for a network failure; null when it did not fail. */
  errorStatus: number | null;
  organizations: ConsentOrganization[] | null;
}): ConsentView {
  if (!s.requestId) return "invalid";
  if (!s.bootstrapped) return "loading";
  if (!s.authenticated) return "sign_in";
  if (s.errorStatus !== null) {
    if (s.errorStatus === 404) return "expired";
    if (s.errorStatus === 403) return "forbidden";
    return "failed";
  }
  if (s.loading || s.organizations === null) return "loading";
  return s.organizations.length === 0 ? "no_organization" : "ready";
}

/** The organization the decision is for: the chosen one, or the only one. */
export function selectedOrganization(
  organizations: ConsentOrganization[],
  chosen: string,
): ConsentOrganization | undefined {
  return (
    organizations.find((o) => o.uuid === chosen) ??
    (organizations.length === 1 ? organizations[0] : undefined)
  );
}

/** Write is offered only when the client asked for it and the member may give it there. */
export function writeOffered(
  writeRequested: boolean,
  organization: ConsentOrganization | undefined,
) {
  return Boolean(writeRequested && organization?.can_write);
}

export type DecideBody =
  | { decision: "deny" }
  | { decision: "approve"; organization_uuid: string; allow_write: boolean };

/**
 * The body of the decision, or null when approving is not possible (no
 * organization chosen). Write is never granted unless it was offered and the
 * member ticked it.
 */
export function decideBody(input: {
  decision: "approve" | "deny";
  organization: ConsentOrganization | undefined;
  writeRequested: boolean;
  allowWrite: boolean;
}): DecideBody | null {
  if (input.decision === "deny") return { decision: "deny" };
  if (!input.organization) return null;
  return {
    decision: "approve",
    organization_uuid: input.organization.uuid,
    allow_write:
      writeOffered(input.writeRequested, input.organization) &&
      input.allowWrite,
  };
}

/** Message key for a decision the API refused. */
export function decideErrorKey(status: number | null) {
  if (status === 404) return "mcp.consent.expired";
  if (status === 403) return "mcp.consent.forbidden";
  return "mcp.consent.failed";
}

/**
 * Where the browser may be sent after a decision. The API builds the address
 * from the client's registered redirect URI; the page checks it again and
 * follows only what a redirect URI can be: `https:`, or `http:` on a loopback
 * address (a program on the member's own computer). Anything else
 * (`javascript:`, `data:`, plain `http:` to a website) is refused.
 */
export function safeClientRedirect(
  raw: string | null | undefined,
): string | null {
  if (!raw) return null;
  let url: URL;
  try {
    url = new URL(raw);
  } catch {
    return null;
  }
  if (url.username || url.password) return null;
  if (url.protocol === "https:") return url.toString();
  if (url.protocol === "http:") {
    const host = url.hostname;
    return host === "localhost" || host === "127.0.0.1" || host === "[::1]"
      ? url.toString()
      : null;
  }
  return null;
}

/** A lifetime in whole hours or days, as the screen words it. */
export function lifetime(
  seconds: number | null | undefined,
): { unit: "hours" | "days"; count: number } | null {
  if (!seconds || !Number.isFinite(seconds) || seconds <= 0) return null;
  const hours = Math.round(seconds / 3600);
  return hours >= 48
    ? { unit: "days", count: Math.round(hours / 24) }
    : { unit: "hours", count: Math.max(1, hours) };
}

/**
 * One decision per request. The buttons are disabled while a decision is on
 * its way, but two clicks can land in the same frame before React re-renders;
 * the gate refuses the second. A refused decision (API error) opens the gate
 * again so the member can try once more.
 */
export function decisionGate() {
  let sent = false;
  return {
    /** True for the first call; false until `reset`. */
    take(): boolean {
      if (sent) return false;
      sent = true;
      return true;
    },
    reset() {
      sent = false;
    },
  };
}
