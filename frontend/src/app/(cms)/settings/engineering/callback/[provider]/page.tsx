"use client";

import { notFound, useParams } from "next/navigation";

import {
  IntegrationOAuthCallback,
  isEngineeringProvider,
} from "@/features/engineering";
import { ActiveOrganizationGate } from "@/features/tenant";

export default function Page() {
  const { provider } = useParams<{ provider: string }>();
  if (!isEngineeringProvider(provider)) notFound();
  return (
    <ActiveOrganizationGate>
      <IntegrationOAuthCallback provider={provider} />
    </ActiveOrganizationGate>
  );
}
