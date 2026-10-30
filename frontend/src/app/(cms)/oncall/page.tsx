import { Suspense } from "react";

import { OncallPage } from "@/features/oncall/components/oncall-page";

export default function OncallRoute() {
  // The tab lives in `?tab=` (useSearchParams needs a Suspense boundary).
  return (
    <Suspense>
      <OncallPage />
    </Suspense>
  );
}
