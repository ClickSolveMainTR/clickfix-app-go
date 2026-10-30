"use client";

import { ActiveOrganizationGate } from "@/features/tenant";
import { TeamsPage } from "@/features/teams";

export default function Page() {
  return (
    <ActiveOrganizationGate>
      <TeamsPage />
    </ActiveOrganizationGate>
  );
}
