import type {
  ChannelCredentials,
  ChannelInstance,
  ChannelSettings,
} from "@/features/channels/types";

/**
 * E-mail channel form (CF-56, CF-135). Keys are the backend credential and
 * setting names (`backend/internal/platform/channels/email`), never renamed
 * here. Form values are strings; settings are converted on the way out.
 *
 * - `resend`: mail is forwarded to the routing address, replies go out
 *   through ClickFix (Resend).
 * - `smtp`: forwarded like `resend`, replies go out through the customer's
 *   SMTP server.
 * - `imap`: ClickFix reads the customer's own mailbox over IMAP and replies
 *   through the same mailbox's SMTP server.
 */
export type EmailProvider = "resend" | "smtp" | "imap";

export type EmailValues = Record<string, string>;

const SENDER_KEYS = ["from_address", "from_name"];
export const SMTP_KEYS = [
  "smtp_host",
  "smtp_port",
  "smtp_username",
  "smtp_password",
  "smtp_tls",
];
export const IMAP_KEYS = [
  "imap_host",
  "imap_port",
  "imap_username",
  "imap_password",
  "imap_tls",
  "imap_folder",
];
/** Write-only credentials: the API returns them masked, so forms start empty. */
export const EMAIL_SECRETS = new Set(["smtp_password", "imap_password"]);

/** Default initial import window of IMAP (`imap_sync_days`). */
export const IMAP_DEFAULT_SYNC_DAYS = 3;

export function emailProviderOf(values: EmailValues): EmailProvider {
  const p = values.provider;
  return p === "smtp" || p === "imap" ? p : "resend";
}

function credentialKeys(provider: EmailProvider) {
  if (provider === "imap") return [...SENDER_KEYS, ...SMTP_KEYS, ...IMAP_KEYS];
  if (provider === "smtp") return [...SENDER_KEYS, ...SMTP_KEYS];
  return SENDER_KEYS;
}

/** Credentials of the selected provider; empty values are left out. */
export function emailCredentials(values: EmailValues): ChannelCredentials {
  const provider = emailProviderOf(values);
  const out: Record<string, string> = { provider };
  for (const key of credentialKeys(provider)) {
    const raw = values[key] ?? "";
    const v = EMAIL_SECRETS.has(key) ? raw : raw.trim();
    if (v !== "") out[key] = v;
  }
  return out as ChannelCredentials;
}

/** Instance settings of the form: routing alias or IMAP import options. */
export function emailSettings(values: EmailValues): ChannelSettings {
  const out: Record<string, unknown> = {};
  if (emailProviderOf(values) !== "imap") {
    const alias = values.inbound_alias?.trim();
    if (alias) out.inbound_alias = alias;
    return out;
  }
  const days = values.imap_sync_days?.trim();
  if (days) out.imap_sync_days = Number(days);
  out.imap_mark_read = values.imap_mark_read === "true";
  out.imap_move_folder = values.imap_move_folder?.trim() ?? "";
  return out;
}

/**
 * Whether the form has what the provider needs to connect (the API checks
 * the rest). `stored` are the credential hints of a saved instance: a stored
 * password stays when its field is left empty.
 */
export function emailComplete(
  values: EmailValues,
  stored: Record<string, string> = {},
) {
  const provider = emailProviderOf(values);
  if (provider === "resend") return true;
  const has = (key: string) =>
    (values[key] ?? "").trim() !== "" ||
    (EMAIL_SECRETS.has(key) && !!stored[key]);
  // SMTP login is optional (relay servers); the IMAP login is not.
  const required = ["from_address", "smtp_host"];
  if (provider === "imap") {
    required.push("imap_host", "imap_username", "imap_password");
  }
  return required.every(has);
}

/**
 * The connection test sends the form as typed, so it needs every stored
 * password typed again (`hints` of a saved instance): secrets never come
 * back to the browser.
 */
export function emailTestReady(
  values: EmailValues,
  hints: Record<string, string> = {},
) {
  if (emailProviderOf(values) === "resend" || !emailComplete(values)) {
    return false;
  }
  return credentialKeys(emailProviderOf(values))
    .filter((key) => EMAIL_SECRETS.has(key))
    .every((key) => !hints[key] || !!values[key]);
}

/** Form values of a saved instance: non-secret credentials and settings. */
export function emailValuesOf(channel: ChannelInstance): EmailValues {
  const out: EmailValues = {};
  for (const [key, value] of Object.entries(channel.credential_hints)) {
    if (!EMAIL_SECRETS.has(key)) out[key] = value;
  }
  out.provider = emailProviderOf(out);
  const s = channel.settings as Record<string, unknown>;
  if (typeof s.inbound_alias === "string") out.inbound_alias = s.inbound_alias;
  if (typeof s.imap_sync_days === "number") {
    out.imap_sync_days = String(s.imap_sync_days);
  }
  if (typeof s.imap_mark_read === "boolean") {
    out.imap_mark_read = String(s.imap_mark_read);
  }
  if (typeof s.imap_move_folder === "string") {
    out.imap_move_folder = s.imap_move_folder;
  }
  return out;
}
