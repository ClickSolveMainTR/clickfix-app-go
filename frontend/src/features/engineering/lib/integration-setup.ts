// Pure rules of the integration settings screen. Type-only imports keep
// this module loadable on its own (see integration-setup.test.mjs).
import type {
  EngineeringIntegration,
  EngineeringIntegrationConfig,
  EngineeringIntegrationInput,
  EngineeringProvider,
} from "@/features/engineering/services/engineering.service";

/** Order of the cards on the settings screen. */
export const ENGINEERING_PROVIDERS: EngineeringProvider[] = [
  "jira",
  "linear",
  "github",
];

export function isEngineeringProvider(
  value: string,
): value is EngineeringProvider {
  return (ENGINEERING_PROVIDERS as string[]).includes(value);
}

/**
 * What the API tells about a provider:
 * - `not_configured`: nothing saved yet
 * - `disabled`: saved but switched off (`enabled: false`); nothing syncs
 * - `needs_auth`: saved and on, no access token yet (OAuth not finished)
 * - `connected`: on, with a stored access token
 */
export type IntegrationState =
  "not_configured" | "disabled" | "needs_auth" | "connected";

export function integrationState(
  integration: EngineeringIntegration | undefined,
): IntegrationState {
  if (!integration) return "not_configured";
  if (!integration.enabled) return "disabled";
  return integration.connected ? "connected" : "needs_auth";
}

export const integrationStateTone = {
  not_configured: "default",
  disabled: "warning",
  needs_auth: "warning",
  connected: "success",
} as const;

/** Plain config fields of each provider, in form order. */
type ConfigField = {
  key:
    | "team_id"
    | "project_id"
    | "cloud_id"
    | "repository"
    | "site_url"
    | "issue_type";
  required: boolean;
};

export const PROVIDER_FIELDS: Record<EngineeringProvider, ConfigField[]> = {
  jira: [
    { key: "site_url", required: true },
    { key: "cloud_id", required: true },
    { key: "project_id", required: true },
    { key: "issue_type", required: false },
  ],
  linear: [
    { key: "team_id", required: true },
    { key: "project_id", required: false },
  ],
  github: [{ key: "repository", required: true }],
};

/** Linear and Jira map each local status to a provider state ID. */
export function hasStateMapping(provider: EngineeringProvider) {
  return provider !== "github";
}

/** Jira also needs the transition that moves an issue into each state. */
export function hasTransitionMapping(provider: EngineeringProvider) {
  return provider === "jira";
}

/** Same shapes the API validates (`repository`, HTTPS URLs, 16+ byte secret). */
const REPOSITORY = /^[A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+$/;
export const WEBHOOK_SECRET_MIN = 16;

export function isHttpsUrl(raw: string): boolean {
  try {
    const url = new URL(raw);
    return url.protocol === "https:" && url.host !== "" && !url.username;
  } catch {
    return false;
  }
}

/**
 * Redirect URI to register in the provider's OAuth app: this panel's
 * callback page. The API accepts HTTPS only, so a plain-HTTP origin (local
 * development) has no suggestion and the field starts empty.
 */
export function suggestedRedirectUri(
  origin: string,
  callbackPath: string,
): string {
  const uri = `${origin}${callbackPath}`;
  return isHttpsUrl(uri) ? uri : "";
}

/** Webhook path of a saved integration; the API host is the customer's own. */
export function webhookPath(integration: EngineeringIntegration): string {
  return `/v1/webhooks/engineering/${integration.uuid}`;
}

export type IntegrationDraft = {
  config: Record<ConfigField["key"] | "redirect_uri", string>;
  states: Record<string, string>;
  transitions: Record<string, string>;
  clientId: string;
  clientSecret: string;
  webhookSecret: string;
};

export function draftOf(
  integration: EngineeringIntegration | undefined,
  suggestedRedirect: string,
): IntegrationDraft {
  const config = integration?.config ?? {};
  return {
    config: {
      team_id: config.team_id ?? "",
      project_id: config.project_id ?? "",
      cloud_id: config.cloud_id ?? "",
      repository: config.repository ?? "",
      site_url: config.site_url ?? "",
      issue_type: config.issue_type ?? "",
      redirect_uri: config.redirect_uri || suggestedRedirect,
    },
    states: { ...(config.states ?? {}) },
    transitions: { ...(config.transitions ?? {}) },
    clientId: "",
    clientSecret: "",
    webhookSecret: "",
  };
}

/** Field keys (`engineering.integrations.invalid.*`) that fail validation. */
export function validateDraft(
  provider: EngineeringProvider,
  draft: IntegrationDraft,
): string[] {
  const problems: string[] = [];
  for (const field of PROVIDER_FIELDS[provider]) {
    if (field.required && !draft.config[field.key].trim()) {
      problems.push(field.key);
    }
  }
  const repository = draft.config.repository.trim();
  if (provider === "github" && repository && !REPOSITORY.test(repository)) {
    problems.push("repository");
  }
  const site = draft.config.site_url.trim();
  if (provider === "jira" && site && !isHttpsUrl(site)) {
    problems.push("site_url");
  }
  const redirect = draft.config.redirect_uri.trim();
  if (redirect && !isHttpsUrl(redirect)) problems.push("redirect_uri");
  const secret = draft.webhookSecret.trim();
  if (secret && secret.length < WEBHOOK_SECRET_MIN) {
    problems.push("webhook_secret");
  }
  return [...new Set(problems)];
}

function compact(map: Record<string, string>): Record<string, string> {
  const out: Record<string, string> = {};
  for (const [key, value] of Object.entries(map)) {
    if (value.trim()) out[key] = value.trim();
  }
  return out;
}

/**
 * Body of the PUT. The API replaces the whole config, so every field of the
 * provider is sent; secrets left empty are omitted and stay as stored.
 */
export function inputOf(
  provider: EngineeringProvider,
  draft: IntegrationDraft,
  enabled: boolean,
): EngineeringIntegrationInput {
  const config: EngineeringIntegrationConfig = {
    redirect_uri: draft.config.redirect_uri.trim(),
  };
  for (const field of PROVIDER_FIELDS[provider]) {
    config[field.key] = draft.config[field.key].trim();
  }
  if (hasStateMapping(provider)) config.states = compact(draft.states);
  if (hasTransitionMapping(provider)) {
    config.transitions = compact(draft.transitions);
  }
  const credentials = compact({
    client_id: draft.clientId,
    client_secret: draft.clientSecret,
    webhook_secret: draft.webhookSecret,
  });
  return {
    config,
    enabled,
    ...(Object.keys(credentials).length > 0 ? { credentials } : {}),
  };
}

/** Consent hosts the API uses; anything else is not followed. */
const AUTHORIZE_HOSTS: Record<EngineeringProvider, string> = {
  linear: "linear.app",
  github: "github.com",
  jira: "auth.atlassian.com",
};

export function isProviderAuthorizeUrl(
  provider: EngineeringProvider,
  raw: string | undefined,
): raw is string {
  if (!raw) return false;
  try {
    const url = new URL(raw);
    return url.protocol === "https:" && url.host === AUTHORIZE_HOSTS[provider];
  } catch {
    return false;
  }
}

/** Validation messages of the API, mapped to `...errors.*` keys. */
const API_MESSAGES: [string, string][] = [
  ["repository must be owner/name", "repository"],
  ["cloud_id is required", "cloud_id"],
  ["redirect_uri must use HTTPS", "redirect_uri"],
  ["site_url must use HTTPS", "site_url"],
  ["webhook_secret must contain", "webhook_secret"],
  ["unknown local state mapping", "states"],
  ["OAuth client credentials and HTTPS redirect_uri", "oauth_not_ready"],
  ["OAuth state expired or already used", "state_expired"],
  ["invalid OAuth state or code", "state_invalid"],
];

/** The fields of the panel's `ApiError` this module reads. */
type ApiFailure = {
  status: number;
  code: string;
  message?: string;
  details?: { message?: string }[];
};

function isApiFailure(error: unknown): error is ApiFailure {
  return (
    typeof error === "object" &&
    error !== null &&
    typeof (error as ApiFailure).status === "number" &&
    typeof (error as ApiFailure).code === "string"
  );
}

/**
 * `engineering.integrations.errors.*` key of a failed call. The API answers
 * with generic English text and never with provider bodies or tokens; the
 * screen shows its own wording. Only `INTEGRATION_UNAVAILABLE` and the HTTP
 * status are stable: validation failures all carry the code `invalid`, so
 * the specific keys match the message text and fall back to `invalid` when
 * the API rewords it.
 */
export function integrationErrorKey(error: unknown): string {
  if (!isApiFailure(error)) return "unknown";
  if (error.code === "INTEGRATION_UNAVAILABLE") return "unavailable";
  if (error.status === 403) return "forbidden";
  if (error.status === 404) return "not_configured";
  const text = [
    error.message ?? "",
    ...(error.details ?? []).map((d) => d.message ?? ""),
  ]
    .join(" ")
    .toLowerCase();
  for (const [needle, key] of API_MESSAGES) {
    if (text.includes(needle.toLowerCase())) return key;
  }
  return error.status === 400 || error.status === 422 ? "invalid" : "unknown";
}
