"use client";

import { Suspense } from "react";

import { DesktopLoginPage } from "@/features/desktop/components/desktop-login-page";

/** Desktop app sign-in in the system browser; `?state=&challenge=` (CF-234). */
export default function DesktopLoginRoute() {
  return (
    <Suspense>
      <DesktopLoginPage />
    </Suspense>
  );
}
