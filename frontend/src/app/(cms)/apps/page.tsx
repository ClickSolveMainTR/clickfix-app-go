"use client";

import { Suspense } from "react";

import { NavAppsPage } from "@/features/sidebar-apps/components/nav-apps-page";
import { ActiveOrganizationGate } from "@/features/tenant";

/** Sidebar apps registered for the navigation placement (CF-118). */
export default function Page() {
  return (
    <ActiveOrganizationGate>
      <Suspense>
        <NavAppsPage />
      </Suspense>
    </ActiveOrganizationGate>
  );
}
