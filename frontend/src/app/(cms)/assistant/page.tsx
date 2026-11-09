"use client";

import { AssistantPage } from "@/features/assistant/components/assistant-page";
import { ActiveOrganizationGate } from "@/features/tenant";

export default function Page() {
  return (
    <ActiveOrganizationGate>
      <AssistantPage />
    </ActiveOrganizationGate>
  );
}
