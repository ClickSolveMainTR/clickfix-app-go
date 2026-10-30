"use client";

import { Suspense } from "react";

import { BillingPortalPage } from "@/features/billing";
import { ActiveOrganizationGate } from "@/features/tenant";

/** Customer billing portal; PayTR / iyzico return here after checkout. */
export default function Page() {
  return (
    <ActiveOrganizationGate>
      <Suspense>
        <BillingPortalPage />
      </Suspense>
    </ActiveOrganizationGate>
  );
}
