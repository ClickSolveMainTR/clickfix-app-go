"use client";

import { ClosingTextsPage } from "@/features/closing-texts";
import { ActiveOrganizationGate } from "@/features/tenant";

export default function ClosingTextsRoute() {
  return (
    <ActiveOrganizationGate>
      <ClosingTextsPage />
    </ActiveOrganizationGate>
  );
}
