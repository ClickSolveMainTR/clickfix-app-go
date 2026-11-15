"use client";

import { useParams } from "next/navigation";

import { ActiveOrganizationGate } from "@/features/tenant";
import { DepartmentDetailPage } from "@/features/departments";

export default function Page() {
  const params = useParams<{ uuid: string }>();
  return (
    <ActiveOrganizationGate>
      <DepartmentDetailPage uuid={params.uuid} />
    </ActiveOrganizationGate>
  );
}
