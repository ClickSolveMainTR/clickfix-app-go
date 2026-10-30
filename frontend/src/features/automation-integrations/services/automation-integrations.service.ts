import type { components } from "@/generated/api";
import { tenantRequest } from "@/lib/api/tenant-request";

export type IntegrationKeyInput = components["schemas"]["IntegrationKeyInput"];
export type IntegrationScope = IntegrationKeyInput["scopes"][number];

/** Row of `GET /v1/tenant/api-keys` (the secret is never listed). */
export type IntegrationKey = {
  uuid: string;
  name: string;
  key_prefix: string;
  scopes: string[];
  rate_limit: number;
  expires_at: string | null;
  revoked_at: string | null;
  last_used_at: string | null;
  created_at: string;
};

/** `POST /v1/tenant/api-keys`: the only response that carries `key`. */
export type IssuedIntegrationKey = {
  uuid: string;
  name: string;
  key: string;
  scopes: string[];
  rate_limit: number;
  expires_at: string | null;
};

/** Row of `GET /v1/tenant/webhooks` (REST hook subscriptions included). */
export type HookSubscription = {
  uuid: string;
  name: string;
  url: string;
  events: string[];
  enabled: boolean;
  created_at: string;
  updated_at: string;
};

type Page<T> = { items: T[]; total: number; limit: number; offset: number };

/**
 * Workflow integrations (CF-97). Keys need tenant.settings.write; hook
 * subscriptions are read with tenant.channels.read and removed with
 * tenant.channels.manage.
 */
export const automationIntegrationsService = {
  keys: (slug: string) =>
    tenantRequest<Page<IntegrationKey>>(slug, "GET", "/v1/tenant/api-keys", {
      query: { limit: 100, offset: 0 },
    }).then((p) => p.items),
  createKey: (slug: string, body: IntegrationKeyInput) =>
    tenantRequest<IssuedIntegrationKey>(slug, "POST", "/v1/tenant/api-keys", {
      body,
    }),
  revokeKey: (slug: string, uuid: string) =>
    tenantRequest<{ revoked: boolean }>(
      slug,
      "DELETE",
      `/v1/tenant/api-keys/${uuid}`,
    ),

  hooks: (slug: string) =>
    tenantRequest<Page<HookSubscription>>(slug, "GET", "/v1/tenant/webhooks", {
      query: { limit: 100, offset: 0 },
    }).then((p) => p.items),
  removeHook: (slug: string, uuid: string) =>
    tenantRequest<{ deleted: boolean }>(
      slug,
      "DELETE",
      `/v1/tenant/webhooks/${uuid}`,
    ),
};
