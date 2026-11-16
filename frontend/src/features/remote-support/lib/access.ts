/**
 * Who sees what in the agent console (CF-237). Pure functions over the
 * caller's permissions and the session state, unit tested; the API enforces
 * the same rules (tenant.remote_support.* and the LiveKit grant).
 */

export type RemoteSupportPermissions = {
  /** tenant.remote_support.use */
  use: boolean;
  /** tenant.remote_support.control */
  control: boolean;
};

export type Mode = "view" | "control";

/** The conversation action: hidden without `use`; control mode needs `control`. */
export function remoteSupportAction(p: RemoteSupportPermissions): {
  visible: boolean;
  modes: Mode[];
} {
  if (!p.use) return { visible: false, modes: [] };
  return { visible: true, modes: p.control ? ["view", "control"] : ["view"] };
}

export type SessionState = {
  status: string;
  mode_requested: Mode;
  control_active: boolean;
};

const OPEN = new Set(["pending", "joined", "active"]);

export function isOpen(status: string) {
  return OPEN.has(status);
}

/**
 * Input may be sent only when every gate is open: the agent holds the
 * control permission, the session asked for control and the customer's
 * consent is in force, the session is live, and LiveKit currently lets the
 * agent publish data (the server flips it at once when control is revoked).
 */
export function controlAllowed(
  p: RemoteSupportPermissions,
  s: SessionState | null | undefined,
  canPublishData: boolean,
): boolean {
  if (!p.use || !p.control || !s) return false;
  return (
    s.mode_requested === "control" &&
    s.control_active &&
    s.status === "active" &&
    canPublishData
  );
}

export type Step = "waiting" | "joined" | "consented" | "active" | "ended";

export const STEPS: readonly Step[] = [
  "waiting",
  "joined",
  "consented",
  "active",
  "ended",
];

/** Status panel position of a session. */
export function sessionStep(s: {
  status: string;
  view_consent_at?: string | null;
}): Step {
  switch (s.status) {
    case "pending":
      return "waiting";
    case "joined":
      return s.view_consent_at ? "consented" : "joined";
    case "active":
      return "active";
    default:
      return "ended";
  }
}

/** 123456789 → "123 456 789" (how the customer message shows it). */
export function formatCode(code: string): string {
  const digits = code.replace(/\D/g, "");
  return digits.replace(/(\d{3})(?=\d)/g, "$1 ").trim();
}

/** Whole seconds → "m:ss" or "h:mm:ss". */
export function formatDuration(totalSeconds: number): string {
  const s = Math.max(0, Math.floor(totalSeconds));
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const sec = String(s % 60).padStart(2, "0");
  return h > 0 ? `${h}:${String(m).padStart(2, "0")}:${sec}` : `${m}:${sec}`;
}

const KNOWN_ERRORS = new Set([
  "REMOTE_SUPPORT_DISABLED",
  "REMOTE_SUPPORT_SUSPENDED",
  "REMOTE_SUPPORT_NOT_CONFIGURED",
  "REMOTE_SUPPORT_CONFLICT",
  "REMOTE_SUPPORT_CONSENT_REQUIRED",
  "REMOTE_SUPPORT_ENDED",
  "REMOTE_SUPPORT_MINUTES_EXHAUSTED",
  "REMOTE_SUPPORT_CODE_INVALID",
  "ENTITLEMENT_REQUIRED",
  "FORBIDDEN",
  "RATE_LIMITED",
]);

/** API error → remote_support.errors.* key suffix. */
export function errorKey(err: unknown): string {
  const code =
    err && typeof err === "object" && "code" in err
      ? String((err as { code?: unknown }).code)
      : "";
  return KNOWN_ERRORS.has(code) ? code : "UNKNOWN";
}
