"use client";

import { ActiveOrganizationGate } from "@/features/tenant";
import { TeamCreatePage } from "@/features/teams";

export default function Page() {
  return (
    <ActiveOrganizationGate>
      <TeamCreatePage />
    </ActiveOrganizationGate>
  );
}
