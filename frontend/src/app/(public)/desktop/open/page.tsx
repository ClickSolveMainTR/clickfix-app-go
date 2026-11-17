"use client";

import { Suspense } from "react";

import { DesktopOpenPage } from "@/features/desktop/components/desktop-open-page";

/** "Masaüstünde aç" link of notification e-mails; `?path=` (CF-236). */
export default function DesktopOpenRoute() {
  return (
    <Suspense>
      <DesktopOpenPage />
    </Suspense>
  );
}
