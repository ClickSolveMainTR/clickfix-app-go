"use client";

import { useQuery } from "@tanstack/react-query";

import { Permission } from "@/config/permissions";
import { useActiveOrganization } from "@/features/organizations";

import {
  sidebarAppsService,
  type SidebarAppPlacement,
} from "../services/sidebar-apps.service";

export const sidebarAppKeys = {
  all: (org: string | undefined) => ["tenant", org, "sidebar-apps"] as const,
};

/**
 * Registered sidebar apps of the active organization. Discovery needs
 * tenant.conversations.read like the backend; without it the hosts render
 * nothing.
 */
export function useSidebarApps() {
  const { active: org } = useActiveOrganization();
  const canRead = (org?.permissions ?? []).includes(
    Permission.TenantConversationsRead,
  );
  return useQuery({
    queryKey: sidebarAppKeys.all(org?.slug),
    queryFn: sidebarAppsService.list,
    enabled: Boolean(org) && canRead,
    staleTime: 60_000,
  });
}

/** Enabled apps for one placement, in registry order. */
export function usePlacedApps(placement: SidebarAppPlacement) {
  const apps = useSidebarApps();
  return {
    ...apps,
    data: (apps.data ?? []).filter(
      (a) => a.enabled && a.placement === placement,
    ),
  };
}
