"use client";

import { ActiveOrganizationGate } from "@/features/tenant";
import { TenantSettingsPage } from "@/features/tenant-settings";

export default function Page() {
  return (
    <ActiveOrganizationGate>
      <TenantSettingsPage />
    </ActiveOrganizationGate>
  );
}
