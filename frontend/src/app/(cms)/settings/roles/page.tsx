"use client";

import { ActiveOrganizationGate } from "@/features/tenant";
import { TenantRolesPage } from "@/features/tenant-roles";

export default function Page() {
  return (
    <ActiveOrganizationGate>
      <TenantRolesPage />
    </ActiveOrganizationGate>
  );
}
