"use client";

import { ActiveOrganizationGate } from "@/features/tenant";
import { PrivacyPage } from "@/features/compliance/privacy-page";

export default function Page() {
  return (
    <ActiveOrganizationGate>
      <PrivacyPage />
    </ActiveOrganizationGate>
  );
}
