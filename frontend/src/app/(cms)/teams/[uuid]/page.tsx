"use client";

import { useParams } from "next/navigation";

import { ActiveOrganizationGate } from "@/features/tenant";
import { TeamDetailPage } from "@/features/teams";

export default function Page() {
  const params = useParams<{ uuid: string }>();
  return (
    <ActiveOrganizationGate>
      <TeamDetailPage uuid={params.uuid} />
    </ActiveOrganizationGate>
  );
}
