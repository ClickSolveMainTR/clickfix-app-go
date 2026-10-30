"use client";

import { RulesPage } from "@/features/automation-rules/components/rules-page";
import { ActiveOrganizationGate } from "@/features/tenant";

/** Automation rules and run log (CF-94). */
export default function Page() {
  return (
    <ActiveOrganizationGate>
      <RulesPage />
    </ActiveOrganizationGate>
  );
}
