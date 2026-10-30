"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import type { components } from "@/generated/api";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { platformRequest } from "@/lib/api/platform-request";
import { tenantRequest } from "@/lib/api/tenant-request";
import type { MeOrganization } from "@/lib/auth/types";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

type Schemas = components["schemas"];

export type SupportGrant = Schemas["SupportGrant"];
export type SupportGrantPage = Schemas["SupportGrantPage"];
export type SupportGrantInput = Schemas["SupportGrantInput"];
export type SupportMode = SupportGrant["mode"];
export type PlatformSupportGrant = Schemas["PlatformSupportGrant"];

const BASE = "/v1/tenant/support-access";

export const supportAccessService = {
  list(slug: string, params: { limit: number; offset: number }) {
    return tenantRequest<SupportGrantPage>(slug, "GET", BASE, {
      query: params,
    });
  },
  grant(slug: string, body: SupportGrantInput) {
    return tenantRequest<SupportGrant>(slug, "POST", BASE, { body });
  },
  revoke(slug: string, uuid: string) {
    return tenantRequest<SupportGrant>(slug, "POST", `${BASE}/${uuid}/revoke`);
  },
  /** Organizations a platform support user can enter now. */
  platform() {
    return platformRequest<{ items: PlatformSupportGrant[] }>(
      "GET",
      "/v1/platform/support-access",
    );
  },
};

export const supportAccessKeys = {
  all: (slug: string) => ["tenant", slug, "support-access"] as const,
  list: (slug: string, params: Record<string, unknown>) =>
    [...supportAccessKeys.all(slug), "list", params] as const,
  platform: ["platform", "support-access"] as const,
};

export function useSupportGrants(params: { limit: number; offset: number }) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: supportAccessKeys.list(slug, params),
    queryFn: () => supportAccessService.list(slug, params),
    placeholderData: (previous) => previous,
    enabled: Boolean(slug),
  });
}

function useInvalidate() {
  const { slug } = useTenant();
  const queryClient = useQueryClient();
  return () =>
    void queryClient.invalidateQueries({
      queryKey: supportAccessKeys.all(slug),
    });
}

export function useGrantSupportAccess() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidate();
  return useMutation({
    mutationFn: (body: SupportGrantInput) =>
      supportAccessService.grant(slug, body),
    onSuccess: () => {
      appToast.success(t("support_access.toast.granted"));
      invalidate();
    },
  });
}

export function useRevokeSupportAccess() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidate();
  return useMutation({
    mutationFn: (uuid: string) => supportAccessService.revoke(slug, uuid),
    onSuccess: () => {
      appToast.success(t("support_access.toast.revoked"));
      invalidate();
    },
  });
}

export function usePlatformSupportGrants() {
  return useQuery({
    queryKey: supportAccessKeys.platform,
    queryFn: () => supportAccessService.platform(),
    refetchInterval: 60_000,
  });
}

/** Static label keys (no template keys, see check-i18n). */
export const MODE_LABEL_KEYS: Record<SupportMode, string> = {
  read_only: "support_access.modes.read_only",
  full: "support_access.modes.full",
};

export const STATUS_LABEL_KEYS: Record<SupportGrant["status"], string> = {
  active: "support_access.status.active",
  expired: "support_access.status.expired",
  revoked: "support_access.status.revoked",
};

/**
 * Mode of a support membership from `/me`: the entry has no mode field, so
 * a permission set of only `.read` / `.read_all` slugs means read-only.
 */
export function supportModeOf(org: MeOrganization): SupportMode {
  return org.permissions.every((p) => /\.(read|read_all)$/.test(p))
    ? "read_only"
    : "full";
}

/** "2 sa 15 dk" style remaining time until `iso` (rounded to minutes). */
export function remaining(
  t: (key: string, vars?: Record<string, string | number>) => string,
  iso: string,
  now: number,
): string {
  const minutes = Math.max(0, Math.round((Date.parse(iso) - now) / 60_000));
  const hours = Math.floor(minutes / 60);
  const rest = minutes % 60;
  return hours
    ? t("support_access.remaining_hours", { hours, minutes: rest })
    : t("support_access.remaining_minutes", { minutes: rest });
}
