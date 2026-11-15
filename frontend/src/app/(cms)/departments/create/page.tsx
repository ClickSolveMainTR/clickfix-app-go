"use client";

import { ActiveOrganizationGate } from "@/features/tenant";
import { DepartmentCreatePage } from "@/features/departments";

export default function Page() {
  return (
    <ActiveOrganizationGate>
      <DepartmentCreatePage />
    </ActiveOrganizationGate>
  );
}
