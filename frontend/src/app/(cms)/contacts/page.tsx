"use client";

import { Suspense } from "react";

import { ContactsPage } from "@/features/contacts";

export default function Page() {
  return (
    <Suspense>
      <ContactsPage />
    </Suspense>
  );
}
