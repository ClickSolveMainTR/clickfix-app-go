"use client";

import { ActiveOrganizationGate } from "@/features/tenant";
import { DepartmentsPage } from "@/features/departments";

export default function Page() {
  return (
    <ActiveOrganizationGate>
      <DepartmentsPage />
    </ActiveOrganizationGate>
  );
}
