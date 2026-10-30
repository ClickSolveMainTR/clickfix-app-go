import { Permission } from "@/config/permissions";
import { useActiveOrganization } from "@/features/organizations";

import type { VideoAvailability } from "../services/video.service";

/** Active organization slug and the video permissions it grants. */
export function useVideoScope() {
  const { active } = useActiveOrganization();
  const perms = active?.permissions ?? [];
  return {
    slug: active?.slug ?? "",
    canUse: perms.includes(Permission.TenantVideoUse),
    canManage: perms.includes(Permission.TenantVideoManage),
  };
}

export const videoKeys = {
  availability: (slug: string) => ["video", slug, "availability"] as const,
  rooms: (slug: string, status: string) =>
    ["video", slug, "rooms", status] as const,
  room: (slug: string, uuid: string) => ["video", slug, "room", uuid] as const,
  participants: (slug: string, uuid: string) =>
    ["video", slug, "participants", uuid] as const,
  recordings: (slug: string, room?: string) =>
    ["video", slug, "recordings", room ?? "all"] as const,
  settings: (slug: string) => ["video", slug, "settings"] as const,
  packages: (slug: string) => ["video", slug, "packages"] as const,
};

/** Maps an API error code to a video.errors.* key. */
export function videoErrorKey(err: unknown): string {
  const code =
    err && typeof err === "object" && "code" in err
      ? String((err as { code?: unknown }).code)
      : "";
  return code.startsWith("VIDEO_") || code === "PAYMENT_GATEWAY_NOT_CONFIGURED"
    ? code
    : "UNKNOWN";
}

/**
 * `video.availability.*` key explaining why rooms cannot be opened (or that
 * minutes run low); "" when everything is fine.
 */
export function availabilityKey(a: VideoAvailability): string {
  if (!a.configured) return "not_configured";
  if (!a.platform_enabled) return "platform_disabled";
  if (!a.entitled) return "not_entitled";
  if (!a.org_enabled) return "org_disabled";
  if (!a.unlimited && a.balance <= 0) return "no_minutes";
  if (a.low_balance) return "low_balance";
  return "";
}

/** Message key of each availabilityKey() value. */
export const availabilityLabel: Record<string, string> = {
  not_configured: "video.availability.not_configured",
  platform_disabled: "video.availability.platform_disabled",
  not_entitled: "video.availability.not_entitled",
  org_disabled: "video.availability.org_disabled",
  no_minutes: "video.availability.no_minutes",
  low_balance: "video.availability.low_balance",
};
