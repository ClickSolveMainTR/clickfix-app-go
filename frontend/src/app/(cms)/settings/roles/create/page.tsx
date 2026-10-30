"use client";

import { ActiveOrganizationGate } from "@/features/tenant";
import { TenantRoleCreatePage } from "@/features/tenant-roles";

export default function Page() {
  return (
    <ActiveOrganizationGate>
      <TenantRoleCreatePage />
    </ActiveOrganizationGate>
  );
}
