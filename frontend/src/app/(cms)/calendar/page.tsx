"use client";

import { Suspense } from "react";

import { CalendarPage } from "@/features/calendar";
import { ActiveOrganizationGate } from "@/features/tenant";

export default function Page() {
  return (
    <ActiveOrganizationGate>
      <Suspense>
        <CalendarPage />
      </Suspense>
    </ActiveOrganizationGate>
  );
}
