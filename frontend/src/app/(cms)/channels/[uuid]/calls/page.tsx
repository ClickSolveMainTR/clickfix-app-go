"use client";

import { useParams } from "next/navigation";

import { VoiceCallsPage } from "@/features/channels";
import { ActiveOrganizationGate } from "@/features/tenant";

export default function Page() {
  const params = useParams<{ uuid: string }>();
  return (
    <ActiveOrganizationGate>
      <VoiceCallsPage uuid={params.uuid} />
    </ActiveOrganizationGate>
  );
}
