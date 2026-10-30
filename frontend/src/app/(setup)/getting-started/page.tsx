"use client";

import { Suspense } from "react";

import { Skeleton } from "@/components/ui/skeleton";
import { SmartSetupPage } from "@/features/onboarding";

/** AI-assisted set-up and template gallery of the active organization (CF-140). */
export default function SmartSetupRoute() {
  return (
    <Suspense fallback={<Skeleton className="m-8 h-96" />}>
      <SmartSetupPage />
    </Suspense>
  );
}
