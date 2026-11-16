import type { components } from "@/generated/api";
import { apiConfig } from "@/config/api";
import { platformRequest as basePlatformRequest } from "@/lib/api/platform-request";
import { tenantRequest as baseTenantRequest } from "@/lib/api/tenant-request";

export type RemoteSupportSession =
  components["schemas"]["RemoteSupportSession"];
export type RemoteSupportCreated =
  components["schemas"]["RemoteSupportCreated"];
export type RemoteSupportGrant = components["schemas"]["RemoteSupportGrant"];
export type RemoteSupportSettings =
  components["schemas"]["RemoteSupportSettings"];
export type RemoteSupportSettingsInput =
  components["schemas"]["RemoteSupportSettingsInput"];
export type RemoteSupportPlatformOrganization =
  components["schemas"]["RemoteSupportPlatformOrganization"];
export type RemoteSupportPlatformReport =
  components["schemas"]["RemoteSupportPlatformReport"];
export type RemoteSupportResolved =
  components["schemas"]["RemoteSupportResolved"];
export type RemoteSupportCustomerView =
  components["schemas"]["RemoteSupportCustomerView"];

type Options = {
  query?: Record<string, string | number | undefined>;
  body?: unknown;
};

// Writes render their own localized error (remote_support.errors.*).
function tenantRequest<T>(
  slug: string,
  method: string,
  path: string,
  options?: Options,
) {
  return baseTenantRequest<T>(slug, method, path, {
    ...options,
    silent: method !== "GET",
  });
}

function platformRequest<T>(method: string, path: string, options?: Options) {
  return basePlatformRequest<T>(method, path, {
    ...options,
    silent: method !== "GET",
  });
}

const T = "/v1/tenant";

/**
 * Remote support, agent side (CF-233 API, CF-237 UI). Starting and watching
 * need tenant.remote_support.use, input control additionally
 * tenant.remote_support.control, settings tenant.remote_support.settings.
 */
export const remoteSupportService = {
  create(slug: string, conversation: string, mode: "view" | "control") {
    return tenantRequest<RemoteSupportCreated>(
      slug,
      "POST",
      `${T}/conversations/${conversation}/remote-support`,
      { body: { mode } },
    );
  },
  forConversation(slug: string, conversation: string) {
    return tenantRequest<{ items: RemoteSupportSession[] }>(
      slug,
      "GET",
      `${T}/conversations/${conversation}/remote-support`,
    );
  },
  session(slug: string, uuid: string) {
    return tenantRequest<RemoteSupportSession>(
      slug,
      "GET",
      `${T}/remote-support/${uuid}`,
    );
  },
  token(slug: string, uuid: string) {
    return tenantRequest<RemoteSupportGrant>(
      slug,
      "POST",
      `${T}/remote-support/${uuid}/token`,
    );
  },
  end(slug: string, uuid: string) {
    return tenantRequest<RemoteSupportSession>(
      slug,
      "POST",
      `${T}/remote-support/${uuid}/end`,
    );
  },
  settings(slug: string) {
    return tenantRequest<RemoteSupportSettings>(
      slug,
      "GET",
      `${T}/remote-support/settings`,
    );
  },
  saveSettings(slug: string, body: RemoteSupportSettingsInput) {
    return tenantRequest<RemoteSupportSettings>(
      slug,
      "PUT",
      `${T}/remote-support/settings`,
      { body },
    );
  },
};

const P = "/v1/platform/remote-support";

/** Platform abuse controls (platform.remote_support.manage). */
export const platformRemoteSupportService = {
  organizations(params: { state?: string; limit: number; offset: number }) {
    return platformRequest<{
      items: RemoteSupportPlatformOrganization[];
      limit: number;
      offset: number;
    }>("GET", `${P}/organizations`, { query: params });
  },
  setState(
    uuid: string,
    body: { state: "active" | "disabled"; reason?: string },
  ) {
    return platformRequest<RemoteSupportSettings>(
      "PUT",
      `${P}/organizations/${uuid}/state`,
      { body },
    );
  },
  reports(params: { limit: number; offset: number }) {
    return platformRequest<{
      items: RemoteSupportPlatformReport[];
      limit: number;
      offset: number;
    }>("GET", `${P}/reports`, { query: params });
  },
};

export class PublicRemoteSupportError extends Error {
  constructor(
    readonly code: string,
    readonly status: number,
  ) {
    super(code);
  }
}

/** Customer calls (no session), same-origin BFF. Used by the dev customer. */
async function publicRequest<T>(path: string, body: unknown): Promise<T> {
  const base = apiConfig.baseUrl.replace(/\/$/, "");
  const res = await fetch(`${base}/v1/public/remote-support${path}`, {
    method: "POST",
    credentials: "omit",
    headers: { Accept: "application/json", "Content-Type": "application/json" },
    body: JSON.stringify(body),
    cache: "no-store",
  });
  const payload = (await res.json().catch(() => null)) as {
    success?: boolean;
    data?: T;
    error?: { code?: string };
  } | null;
  if (!res.ok || !payload?.success) {
    throw new PublicRemoteSupportError(
      payload?.error?.code ?? "UNKNOWN",
      res.status,
    );
  }
  return payload.data as T;
}

/**
 * The customer side of the API (what ClickFix Destek calls). In this app it
 * is used only by the development helper that stands in for the desktop
 * app until CF-239.
 */
export const publicRemoteSupportService = {
  resolve(code: string) {
    return publicRequest<RemoteSupportResolved>("/resolve", {
      code,
      client_os: "web-dev",
      client_version: "dev",
    });
  },
  consent(
    uuid: string,
    body: {
      client_secret: string;
      view?: true;
      control?: boolean;
      recording?: boolean;
    },
  ) {
    return publicRequest<RemoteSupportCustomerView>(`/${uuid}/consent`, body);
  },
  token(uuid: string, clientSecret: string) {
    return publicRequest<RemoteSupportGrant>(`/${uuid}/token`, {
      client_secret: clientSecret,
    });
  },
  end(uuid: string, clientSecret: string) {
    return publicRequest<RemoteSupportCustomerView>(`/${uuid}/end`, {
      client_secret: clientSecret,
    });
  },
};
