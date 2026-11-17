"use client";

import { useParams } from "next/navigation";
import { Suspense } from "react";

import { ContactPage } from "@/features/contacts";
import { ActiveOrganizationGate } from "@/features/tenant";

export default function Page() {
  const params = useParams<{ uuid: string }>();
  return (
    <ActiveOrganizationGate>
      <Suspense>
        <ContactPage uuid={params.uuid} />
      </Suspense>
    </ActiveOrganizationGate>
  );
}
