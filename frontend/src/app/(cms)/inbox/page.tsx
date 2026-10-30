"use client";

import { Suspense } from "react";

import { InboxPage } from "@/features/inbox";

/** Unified inbox (CF-43). */
export default function Page() {
  return (
    <Suspense>
      <InboxPage />
    </Suspense>
  );
}
