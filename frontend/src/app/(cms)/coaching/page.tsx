import { Suspense } from "react";

import { CoachingPage } from "@/features/coaching/components/coaching-page";

export default function CoachingRoute() {
  // The tab lives in `?tab=` (useSearchParams needs a Suspense boundary).
  return (
    <Suspense>
      <CoachingPage />
    </Suspense>
  );
}
