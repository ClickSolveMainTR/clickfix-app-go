"use client";

import { ConversationStatusesPage } from "@/features/conversation-statuses";
import { ActiveOrganizationGate } from "@/features/tenant";

export default function ConversationStatusesRoute() {
  return (
    <ActiveOrganizationGate>
      <ConversationStatusesPage />
    </ActiveOrganizationGate>
  );
}
