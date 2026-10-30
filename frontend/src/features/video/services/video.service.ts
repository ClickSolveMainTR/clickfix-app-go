import type { components } from "@/generated/api";
import { apiConfig } from "@/config/api";
import { platformRequest as basePlatformRequest } from "@/lib/api/platform-request";
import {
  tenantDownloadFile,
  tenantRequest as baseTenantRequest,
} from "@/lib/api/tenant-request";

export type VideoRoom = components["schemas"]["VideoRoom"];
export type VideoRoomInput = components["schemas"]["VideoRoomInput"];
export type VideoRoomPublic = components["schemas"]["VideoRoomPublic"];
export type VideoJoinGrant = components["schemas"]["VideoJoinGrant"];
export type VideoParticipant = components["schemas"]["VideoParticipant"];
export type VideoLink = components["schemas"]["VideoLink"];
export type VideoRecording = components["schemas"]["VideoRecording"];
export type VideoAvailability = components["schemas"]["VideoAvailability"];
export type VideoPolicy = components["schemas"]["VideoPolicy"];
export type VideoPolicyPartial = components["schemas"]["VideoPolicyPartial"];
export type VideoOrgSettings = components["schemas"]["VideoOrgSettings"];
export type VideoOrgSettingsInput =
  components["schemas"]["VideoOrgSettingsInput"];
export type VideoPlatformSettings =
  components["schemas"]["VideoPlatformSettings"];
export type CreditPackage = components["schemas"]["BillingCreditPackageRecord"];
export type CreditPackageInput =
  components["schemas"]["BillingCreditPackageInput"];

type Page<T> = { items: T[]; total: number; limit: number; offset: number };

const V = "/v1/tenant/video";

type Options = {
  query?: Record<string, string | number | undefined>;
  body?: unknown;
};

// Writes report their own localized error (video.errors.*), so they skip the
// global error toast; reads keep it.
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

/**
 * Video rooms (CF-139). Tenant routes need `tenant.video.use`; settings and
 * recordings `tenant.video.manage`; platform routes `platform.video.manage`.
 */
export const videoService = {
  availability(slug: string) {
    return tenantRequest<VideoAvailability>(slug, "GET", `${V}/availability`);
  },
  rooms(
    slug: string,
    params: { status?: string; limit: number; offset: number },
  ) {
    return tenantRequest<Page<VideoRoom>>(slug, "GET", `${V}/rooms`, {
      query: params,
    });
  },
  room(slug: string, uuid: string) {
    return tenantRequest<VideoRoom>(slug, "GET", `${V}/rooms/${uuid}`);
  },
  create(slug: string, body: VideoRoomInput) {
    return tenantRequest<VideoRoom>(slug, "POST", `${V}/rooms`, { body });
  },
  join(slug: string, uuid: string) {
    return tenantRequest<VideoJoinGrant>(
      slug,
      "POST",
      `${V}/rooms/${uuid}/join`,
    );
  },
  end(slug: string, uuid: string) {
    return tenantRequest<VideoRoom>(slug, "POST", `${V}/rooms/${uuid}/end`);
  },
  link(slug: string, uuid: string, role: "guest" | "agent") {
    return tenantRequest<VideoLink>(slug, "POST", `${V}/rooms/${uuid}/links`, {
      body: { role },
    });
  },
  invite(
    slug: string,
    uuid: string,
    body: { email: string; name?: string; locale?: "tr" | "en" },
  ) {
    return tenantRequest<VideoLink>(slug, "POST", `${V}/rooms/${uuid}/invite`, {
      body,
    });
  },
  participants(slug: string, uuid: string) {
    return tenantRequest<{ items: VideoParticipant[] }>(
      slug,
      "GET",
      `${V}/rooms/${uuid}/participants`,
    );
  },
  participant(
    slug: string,
    uuid: string,
    participant: string,
    action: "admit" | "deny" | "remove",
  ) {
    return tenantRequest<VideoParticipant>(
      slug,
      "POST",
      `${V}/rooms/${uuid}/participants/${participant}/${action}`,
    );
  },
  startRecording(slug: string, uuid: string) {
    return tenantRequest<VideoRecording>(
      slug,
      "POST",
      `${V}/rooms/${uuid}/recording/start`,
    );
  },
  stopRecording(slug: string, uuid: string) {
    return tenantRequest<{ stopping: boolean }>(
      slug,
      "POST",
      `${V}/rooms/${uuid}/recording/stop`,
    );
  },
  recordings(
    slug: string,
    params: { room?: string; limit: number; offset: number },
  ) {
    return tenantRequest<Page<VideoRecording>>(slug, "GET", `${V}/recordings`, {
      query: params,
    });
  },
  deleteRecording(slug: string, uuid: string) {
    return tenantRequest<{ deleted: boolean }>(
      slug,
      "DELETE",
      `${V}/recordings/${uuid}`,
    );
  },
  /** Streams the MP4 through the API and saves it (no bucket URLs). */
  async downloadRecording(slug: string, uuid: string) {
    const { blob, filename } = await tenantDownloadFile(
      slug,
      `${V}/recordings/${uuid}/download`,
    );
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = filename ?? `recording-${uuid}.mp4`;
    a.click();
    URL.revokeObjectURL(url);
  },
  settings(slug: string) {
    return tenantRequest<VideoOrgSettings>(slug, "GET", `${V}/settings`);
  },
  saveSettings(slug: string, body: VideoOrgSettingsInput) {
    return tenantRequest<VideoOrgSettings>(slug, "PUT", `${V}/settings`, {
      body,
    });
  },
  /** Credit packages of every unit; the video screens keep video_minutes. */
  packages(slug: string) {
    return tenantRequest<{ items: CreditPackage[] }>(
      slug,
      "GET",
      "/v1/tenant/billing/credit-packages",
    );
  },
  purchase(slug: string, packageUuid: string) {
    return tenantRequest<{ uuid: string }>(
      slug,
      "POST",
      "/v1/tenant/billing/credit-purchases",
      {
        body: {
          package_uuid: packageUuid,
          idempotency_key: crypto.randomUUID(),
        },
      },
    );
  },
};

export const platformVideoService = {
  settings() {
    return platformRequest<VideoPlatformSettings>(
      "GET",
      "/v1/platform/video/settings",
    );
  },
  save(policy: VideoPolicy) {
    return platformRequest<VideoPlatformSettings>(
      "PUT",
      "/v1/platform/video/settings",
      {
        body: { policy },
      },
    );
  },
  savePlan(planUuid: string, policy: VideoPolicyPartial) {
    return platformRequest<VideoPlatformSettings>(
      "PUT",
      `/v1/platform/video/plans/${planUuid}`,
      {
        body: policy,
      },
    );
  },
  deletePlan(planUuid: string) {
    return platformRequest<VideoPlatformSettings>(
      "DELETE",
      `/v1/platform/video/plans/${planUuid}`,
    );
  },
  packages() {
    return platformRequest<{ items: CreditPackage[] }>(
      "GET",
      "/v1/platform/billing/credit-packages",
    );
  },
  createPackage(body: CreditPackageInput) {
    return platformRequest<CreditPackage>(
      "POST",
      "/v1/platform/billing/credit-packages",
      { body },
    );
  },
};

/** Error code of a failed public call (VIDEO_PIN_INVALID, VIDEO_ROOM_ENDED, ...). */
export class PublicVideoError extends Error {
  constructor(
    readonly code: string,
    readonly status: number,
  ) {
    super(code);
  }
}

/**
 * Guest calls go through the same-origin BFF without a session. Plain fetch:
 * a 401 here (agent link) must not trigger the signed-in auth handlers.
 */
async function publicRequest<T>(
  method: string,
  path: string,
  body?: unknown,
): Promise<T> {
  const base = apiConfig.baseUrl.replace(/\/$/, "");
  const res = await fetch(`${base}${path}`, {
    method,
    credentials: "omit",
    headers: {
      Accept: "application/json",
      ...(body ? { "Content-Type": "application/json" } : {}),
    },
    body: body ? JSON.stringify(body) : undefined,
    cache: "no-store",
  });
  const payload = (await res.json().catch(() => null)) as {
    success?: boolean;
    data?: T;
    error?: { code?: string };
  } | null;
  if (!res.ok || !payload?.success) {
    throw new PublicVideoError(payload?.error?.code ?? "UNKNOWN", res.status);
  }
  return payload.data as T;
}

export const publicVideoService = {
  room(code: string) {
    return publicRequest<VideoRoomPublic>(
      "GET",
      `/v1/public/video/rooms/${encodeURIComponent(code)}`,
    );
  },
  join(code: string, body: { name: string; email?: string; pin?: string }) {
    return publicRequest<VideoJoinGrant>(
      "POST",
      `/v1/public/video/rooms/${encodeURIComponent(code)}/join`,
      body,
    );
  },
  ticket(ticket: string) {
    return publicRequest<VideoJoinGrant>("POST", "/v1/public/video/ticket", {
      ticket,
    });
  },
};
