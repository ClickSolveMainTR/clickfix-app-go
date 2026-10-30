"use client";

import { ChannelsPage } from "@/features/channels";
import { ActiveOrganizationGate } from "@/features/tenant";

export default function Page() {
  return (
    <ActiveOrganizationGate>
      <ChannelsPage />
    </ActiveOrganizationGate>
  );
}
