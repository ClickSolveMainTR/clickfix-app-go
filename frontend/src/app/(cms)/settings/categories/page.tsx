"use client";

import { CategoriesPage } from "@/features/categories";
import { ActiveOrganizationGate } from "@/features/tenant";

export default function CategoriesRoute() {
  return (
    <ActiveOrganizationGate>
      <CategoriesPage />
    </ActiveOrganizationGate>
  );
}
