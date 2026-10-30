"use client";

import { WorkflowsPage } from "@/features/workflows/components/workflows-page";
import { ActiveOrganizationGate } from "@/features/tenant";

/** Visual workflows (CF-96). */
export default function Page() {
  return (
    <ActiveOrganizationGate>
      <WorkflowsPage />
    </ActiveOrganizationGate>
  );
}
