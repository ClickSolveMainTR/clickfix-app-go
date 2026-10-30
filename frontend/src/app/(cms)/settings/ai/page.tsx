import { Suspense } from "react";

import { AISettingsPage } from "@/features/ai-settings/components/ai-settings-page";

export default function AISettingsRoute() {
  // The tab lives in `?tab=` (useSearchParams needs a Suspense boundary).
  return (
    <Suspense>
      <AISettingsPage />
    </Suspense>
  );
}
