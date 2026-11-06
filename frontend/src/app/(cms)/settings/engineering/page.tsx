"use client";

import { EngineeringIntegrationsPage } from "@/features/engineering";
import { ActiveOrganizationGate } from "@/features/tenant";

export default function Page() {
  return (
    <ActiveOrganizationGate>
      <EngineeringIntegrationsPage />
    </ActiveOrganizationGate>
  );
}
