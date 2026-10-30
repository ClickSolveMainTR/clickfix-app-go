"use client";

import { ActiveOrganizationGate } from "@/features/tenant";
import { MembersPage } from "@/features/tenant-members";

export default function Page() {
  return (
    <ActiveOrganizationGate>
      <MembersPage />
    </ActiveOrganizationGate>
  );
}
