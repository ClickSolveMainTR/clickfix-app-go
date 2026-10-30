import type { ChannelInstance } from "@/features/channels/types";

/**
 * Credential forms of the CF-119–123 channels. Keys are the backend adapter
 * credential names (`backend/internal/platform/channels/{sms,voice,teams,
 * googlechat,apichannel}`), never renamed here. Secrets are write-only: the API returns
 * them masked, so edit forms leave them empty to keep the stored value.
 */
export type SetupField = {
  key: string;
  secret?: boolean;
  required?: boolean;
  placeholder?: string;
  /** Random value made in the browser and shown once after connecting. */
  generated?: boolean;
  /** Multi-line value (a key file) pasted or loaded from a file of this type. */
  file?: string;
};

export type SetupProvider = {
  value: string;
  fields: SetupField[];
  /** How the provider reaches ClickFix: plain URL, URL + ?token=, none. */
  webhook: "url" | "token" | "none";
};

const TWILIO: SetupField[] = [
  { key: "account_sid", required: true, placeholder: "AC…" },
  { key: "auth_token", required: true, secret: true },
  { key: "from_number", required: true, placeholder: "+908501234567" },
];

export const SMS_PROVIDERS: SetupProvider[] = [
  {
    value: "netgsm",
    webhook: "token",
    fields: [
      { key: "usercode", required: true, placeholder: "850xxxxxxx" },
      { key: "password", required: true, secret: true },
      { key: "msgheader", required: true },
      { key: "webhook_token", required: true, secret: true, generated: true },
    ],
  },
  {
    value: "iletimerkezi",
    webhook: "token",
    fields: [
      { key: "api_key", required: true, secret: true },
      { key: "api_hash", required: true, secret: true },
      { key: "msgheader", required: true },
      { key: "webhook_token", required: true, secret: true, generated: true },
      { key: "webhook_token", secret: true, generated: true },
    ],
  },
  { value: "twilio", webhook: "url", fields: TWILIO },
];

/** İYS consent check of commercial SMS; optional for every provider. */
export const IYS_FIELDS: SetupField[] = [
  { key: "iys_username" },
  { key: "iys_password", secret: true },
  { key: "iys_code" },
  { key: "iys_brand_code" },
];

export const VOICE_PROVIDERS: SetupProvider[] = [
  { value: "twilio", webhook: "url", fields: TWILIO },
  {
    value: "netgsm",
    webhook: "token",
    fields: [
      { key: "usercode", required: true, secret: true },
      { key: "password", required: true, secret: true },
      { key: "webhook_token", required: true, secret: true, generated: true },
    ],
  },
];

export const TEAMS_FIELDS: SetupField[] = [
  {
    key: "app_id",
    required: true,
    placeholder: "00000000-0000-0000-0000-000000000000",
  },
  {
    key: "tenant_id",
    required: true,
    placeholder: "00000000-0000-0000-0000-000000000000",
  },
  { key: "app_password", required: true, secret: true },
];

/** Google Chat app (CF-137): project number + service account key file. */
export const GOOGLE_CHAT_FIELDS: SetupField[] = [
  { key: "project_number", required: true, placeholder: "123456789012" },
  {
    key: "service_account_json",
    required: true,
    secret: true,
    file: "application/json,.json",
    placeholder: '{ "type": "service_account", … }',
  },
];

export const API_FIELDS: SetupField[] = [
  {
    key: "webhook_url",
    required: true,
    placeholder: "https://example.com/clickfix/replies",
  },
  { key: "signing_secret", required: true, secret: true, generated: true },
];

/** Types whose credential form is described by this file. */
export const SETUP_TYPES = new Set([
  "sms",
  "voice",
  "teams",
  "google_chat",
  "api",
]);

/** Provider list of a type with a provider select, else null. */
export function providersOf(type: string): SetupProvider[] | null {
  if (type === "sms") return SMS_PROVIDERS;
  if (type === "voice") return VOICE_PROVIDERS;
  return null;
}

/** Fields shown for a type (and its selected provider). */
export function fieldsOf(type: string, provider?: string): SetupField[] {
  const providers = providersOf(type);
  if (providers) {
    const p = providers.find((x) => x.value === provider) ?? providers[0];
    return type === "sms" ? [...p.fields, ...IYS_FIELDS] : p.fields;
  }
  if (type === "teams") return TEAMS_FIELDS;
  if (type === "google_chat") return GOOGLE_CHAT_FIELDS;
  if (type === "api") return API_FIELDS;
  return [];
}

/** Webhook style of a type/provider (Teams, Google Chat and API post to the URL). */
export function webhookModeOf(
  type: string,
  provider?: string,
): SetupProvider["webhook"] {
  const providers = providersOf(type);
  if (providers) {
    return (providers.find((x) => x.value === provider) ?? providers[0])
      .webhook;
  }
  return type === "teams" || type === "google_chat" || type === "api"
    ? "url"
    : "none";
}

/** 48 random URL-safe characters (the backend wants 32+). */
export function generateSecret(): string {
  const bytes = new Uint8Array(36);
  crypto.getRandomValues(bytes);
  return btoa(String.fromCharCode(...bytes))
    .replace(/\+/g, "-")
    .replace(/\//g, "_")
    .replace(/=+$/, "");
}

/**
 * URL to paste into the provider console. `webhook_url` is absolute when
 * the API knows PUBLIC_API_URL; otherwise the path is shown as is. Token
 * style providers (İleti Merkezi, Netgsm santral) do not sign callbacks, so
 * the token goes into the query; it is only known right after connecting.
 */
export function providerWebhookUrl(
  channel: Pick<
    ChannelInstance,
    "webhook_url" | "webhook_path" | "shared_webhook_path"
  >,
  token?: string,
): string {
  const base =
    channel.webhook_url ||
    channel.shared_webhook_path ||
    channel.webhook_path ||
    "";
  return token ? `${base}?token=${encodeURIComponent(token)}` : base;
}

/**
 * True when every required field has a value. The İYS check needs all four
 * İYS fields, so they are all-or-nothing, and required for commercial SMS.
 */
export function isComplete(
  type: string,
  values: Record<string, string>,
  commercial = false,
): boolean {
  const filled = (key: string) => (values[key] ?? "").trim() !== "";
  if (
    !fieldsOf(type, values.provider)
      .filter((f) => f.required)
      .every((f) => filled(f.key))
  ) {
    return false;
  }
  if (type !== "sms") return true;
  const iys = IYS_FIELDS.filter((f) => filled(f.key)).length;
  return commercial
    ? iys === IYS_FIELDS.length
    : iys === 0 || iys === IYS_FIELDS.length;
}

/**
 * Credentials to send: only the active provider's fields, trimmed, empty
 * ones dropped (on update the API keeps the stored value for those).
 */
export function credentialsOf(
  type: string,
  values: Record<string, string>,
): Record<string, string> {
  const out: Record<string, string> = {};
  if (providersOf(type)) {
    out.provider = values.provider ?? providersOf(type)![0].value;
  }
  for (const f of fieldsOf(type, out.provider)) {
    const v = (values[f.key] ?? "").trim();
    if (v !== "") out[f.key] = v;
  }
  return out;
}

/** Starting values of a new form: default provider, fresh generated secrets. */
export function initialValues(type: string): Record<string, string> {
  const out: Record<string, string> = {};
  const providers = providersOf(type);
  if (providers) out.provider = providers[0].value;
  for (const f of fieldsOf(type, out.provider)) {
    if (f.generated) out[f.key] = generateSecret();
  }
  return out;
}

/** Path of the public API reference describing the API channel contract. */
export const API_CHANNEL_DOCS_PATH = "/docs/#model/apichannelmessage";

/** API docs URL on the API origin (the browser never calls it, only links). */
export function apiDocsUrl(channel: Pick<ChannelInstance, "webhook_url">) {
  try {
    return new URL(API_CHANNEL_DOCS_PATH, channel.webhook_url).toString();
  } catch {
    return "";
  }
}
