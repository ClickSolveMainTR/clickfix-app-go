"use client";

import { useParams } from "next/navigation";

import { AccountPage } from "@/features/contacts";
import { ActiveOrganizationGate } from "@/features/tenant";

export default function Page() {
  const params = useParams<{ uuid: string }>();
  return (
    <ActiveOrganizationGate>
      <AccountPage uuid={params.uuid} />
    </ActiveOrganizationGate>
  );
}
