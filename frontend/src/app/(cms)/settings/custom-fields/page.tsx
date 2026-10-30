import { Suspense } from "react";

import { CustomFieldsSettingsPage } from "@/features/custom-fields/components/settings/custom-fields-settings-page";

export default function CustomFieldsSettingsRoute() {
  // The tab lives in `?tab=` (useSearchParams needs a Suspense boundary).
  return (
    <Suspense>
      <CustomFieldsSettingsPage />
    </Suspense>
  );
}
