"use client";

import { Suspense } from "react";

import { Skeleton } from "@/components/ui/skeleton";
import { SetupWizardPage } from "@/features/onboarding";

/** Guided set-up of the active organization (CF-13). */
export default function OnboardingPage() {
  return (
    <Suspense fallback={<Skeleton className="mx-auto h-96 w-full max-w-5xl" />}>
      <SetupWizardPage />
    </Suspense>
  );
}
