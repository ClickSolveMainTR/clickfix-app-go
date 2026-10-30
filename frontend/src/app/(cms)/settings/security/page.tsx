"use client";

import { ActiveOrganizationGate } from "@/features/tenant";
import { SecurityPage } from "@/features/security-settings/security-page";

export default function Page() {
  return (
    <ActiveOrganizationGate>
      <SecurityPage />
    </ActiveOrganizationGate>
  );
}
