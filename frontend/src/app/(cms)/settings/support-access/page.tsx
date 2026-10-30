"use client";

import { ActiveOrganizationGate } from "@/features/tenant";
import { SupportAccessPage } from "@/features/support-access/support-access-page";

export default function Page() {
  return (
    <ActiveOrganizationGate>
      <SupportAccessPage />
    </ActiveOrganizationGate>
  );
}
