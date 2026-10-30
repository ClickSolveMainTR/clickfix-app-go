"use client";

import { Suspense } from "react";

import { CompaniesPage } from "@/features/contacts";

export default function Page() {
  return (
    <Suspense>
      <CompaniesPage />
    </Suspense>
  );
}
