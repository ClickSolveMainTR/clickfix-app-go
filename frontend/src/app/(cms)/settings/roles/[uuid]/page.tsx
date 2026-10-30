"use client";

import { useParams } from "next/navigation";

import { ActiveOrganizationGate } from "@/features/tenant";
import { TenantRoleDetailPage } from "@/features/tenant-roles";

export default function Page() {
  const params = useParams<{ uuid: string }>();
  return (
    <ActiveOrganizationGate>
      <TenantRoleDetailPage uuid={params.uuid} />
    </ActiveOrganizationGate>
  );
}
