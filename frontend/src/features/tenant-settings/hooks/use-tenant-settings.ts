"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { tenantSettingsService } from "@/features/tenant-settings/services/tenant-settings.service";
import type {
  OrganizationProfile,
  OrganizationProfilePatchRequest,
} from "@/features/tenant-settings/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantKeys } from "@/features/tenant/services/tenant.service";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/** Query keys carry the organization slug. */
export const tenantSettingsKeys = {
  all: (slug: string) => ["tenant", slug, "organization"] as const,
  profile: (slug: string) =>
    [...tenantSettingsKeys.all(slug), "profile"] as const,
};

export function useOrganizationProfile() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: tenantSettingsKeys.profile(slug),
    queryFn: () => tenantSettingsService.get(slug),
  });
}

/** Stores the new profile and refreshes the shell (sidebar, switcher, `/me`). */
function useProfileRefresh() {
  const { slug } = useTenant();
  const { hydrateProfile } = useAuth();
  const queryClient = useQueryClient();
  return (profile: OrganizationProfile) => {
    queryClient.setQueryData(tenantSettingsKeys.profile(slug), profile);
    void queryClient.invalidateQueries({ queryKey: tenantKeys.context(slug) });
    void hydrateProfile();
  };
}

export function useUpdateOrganizationProfile() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const refresh = useProfileRefresh();
  return useMutation({
    mutationFn: (body: OrganizationProfilePatchRequest) =>
      tenantSettingsService.update(slug, body),
    onSuccess: (profile) => {
      appToast.success(t("tenant_settings.toast.saved"));
      refresh(profile);
    },
  });
}

export function useUploadOrganizationLogo() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const refresh = useProfileRefresh();
  return useMutation({
    mutationFn: (file: File) => tenantSettingsService.uploadLogo(slug, file),
    onSuccess: (profile) => {
      appToast.success(t("tenant_settings.profile.toast.logo_uploaded"));
      refresh(profile);
    },
  });
}

export function useRemoveOrganizationLogo() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const refresh = useProfileRefresh();
  return useMutation({
    mutationFn: () => tenantSettingsService.removeLogo(slug),
    onSuccess: (profile) => {
      appToast.success(t("tenant_settings.profile.toast.logo_removed"));
      refresh(profile);
    },
  });
}
