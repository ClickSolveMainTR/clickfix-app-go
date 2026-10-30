import { Suspense } from "react";

import { WorkforcePage } from "@/features/workforce/components/workforce-page";

export default function WorkforceRoute() {
  // The tab lives in `?tab=` (useSearchParams needs a Suspense boundary).
  return (
    <Suspense>
      <WorkforcePage />
    </Suspense>
  );
}
