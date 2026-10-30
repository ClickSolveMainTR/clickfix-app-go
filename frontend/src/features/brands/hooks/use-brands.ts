"use client";

import { useQuery } from "@tanstack/react-query";

import { Permission } from "@/config/permissions";
import { useActiveOrganization } from "@/features/organizations";

import { brandsService } from "../services/brands.service";

export const brandKeys = {
  all: (org: string | undefined) => ["tenant", org, "brands"] as const,
};

/**
 * Brands of the active organization, shared by the settings page, the
 * channel/widget/article selectors and the inbox filter. Disabled without
 * tenant.channels.read (selectors then simply hide).
 */
export function useBrands() {
  const { active: org } = useActiveOrganization();
  const canRead = (org?.permissions ?? []).includes(
    Permission.TenantChannelsRead,
  );
  return useQuery({
    queryKey: brandKeys.all(org?.slug),
    queryFn: brandsService.list,
    enabled: Boolean(org) && canRead,
    staleTime: 60_000,
  });
}
