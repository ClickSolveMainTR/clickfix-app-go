"use client";

import { SidebarAppsSettingsPage } from "@/features/sidebar-apps/components/sidebar-apps-settings-page";
import { ActiveOrganizationGate } from "@/features/tenant";

/** Sidebar app registry (CF-118). */
export default function Page() {
  return (
    <ActiveOrganizationGate>
      <SidebarAppsSettingsPage />
    </ActiveOrganizationGate>
  );
}
