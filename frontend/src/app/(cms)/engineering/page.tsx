"use client";

import { IssuesPage } from "@/features/engineering";
import { ActiveOrganizationGate } from "@/features/tenant";

export default function Page() {
  return (
    <ActiveOrganizationGate>
      <IssuesPage />
    </ActiveOrganizationGate>
  );
}
