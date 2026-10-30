"use client";

import { ActiveOrganizationGate } from "@/features/tenant";
import { AuditPage } from "@/features/audit/audit-page";

export default function Page() {
  return (
    <ActiveOrganizationGate>
      <AuditPage />
    </ActiveOrganizationGate>
  );
}
