"use client";

import { useParams } from "next/navigation";

import { IssueDetailPage } from "@/features/engineering";
import { ActiveOrganizationGate } from "@/features/tenant";

export default function Page() {
  const params = useParams<{ uuid: string }>();
  return (
    <ActiveOrganizationGate>
      <IssueDetailPage uuid={params.uuid} />
    </ActiveOrganizationGate>
  );
}
