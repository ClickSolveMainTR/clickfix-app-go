/**
 * What the SDK tells the integrator when an identify call fails (CF-176).
 *
 * The texts are fixed: they are built from the error code and HTTP status
 * only, never from the request or the response body, so `user_id`,
 * `user_hash` and the identity secret cannot end up in the console or in an
 * `error` event.
 */
export interface IdentifyFailure {
  code: string;
  message: string;
}

export const IDENTITY_DISABLED = "WIDGET_IDENTITY_DISABLED";
export const IDENTITY_INVALID = "WIDGET_IDENTITY_INVALID";

const PANEL = "Web widget > Security (Güvenlik) > Identity verification";

function safeCode(err: unknown): string {
  const e = err as { code?: unknown; status?: unknown } | null;
  if (e && typeof e.code === "string" && /^[A-Z0-9_]{1,64}$/.test(e.code))
    return e.code;
  if (e && typeof e.status === "number") return "HTTP_" + e.status;
  return err instanceof TypeError ? "NETWORK_ERROR" : "IDENTIFY_FAILED";
}

/**
 * Warning when identify carried company.id but user_hash signed only
 * user_id (CF-206): the user is verified, the company id was not bound.
 */
export function unsignedCompanyWarning(): string {
  return (
    "identify: company.id was sent but user_hash signs only user_id, so the company was not linked by its id " +
    '(name and domain were used as hints). Sign both on your server: hex(HMAC-SHA256(identity_secret, user_id + "\\n" + company.id)).'
  );
}

export function identifyFailure(err: unknown): IdentifyFailure {
  const code = safeCode(err);
  if (code === IDENTITY_DISABLED)
    return {
      code,
      message:
        `identify was rejected (403 ${code}): this widget has no identity secret yet, ` +
        `so the user stays anonymous. Generate the secret in the ClickFix panel under ${PANEL}, ` +
        "then send a user_hash computed on your server.",
    };
  if (code === IDENTITY_INVALID)
    return {
      code,
      message:
        `identify was rejected (401 ${code}): user_hash does not match user_id (and company.id when sent), ` +
        "so the user stays anonymous. Compute hex(HMAC-SHA256(identity_secret, user_id)) on your server, " +
        'or with the company hex(HMAC-SHA256(identity_secret, user_id + "\\n" + company.id)), ' +
        `with the widget's current secret (${PANEL}); if the secret was rotated, update it on your server too.`,
    };
  return {
    code,
    message: `identify failed (${code}): the user stays anonymous.`,
  };
}
