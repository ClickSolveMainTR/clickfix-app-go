"use client";

import { notFound, useParams } from "next/navigation";

import {
  IntegrationOAuthCallback,
  isEngineeringProvider,
} from "@/features/engineering";

export default function Page() {
  const { provider } = useParams<{ provider: string }>();
  if (!isEngineeringProvider(provider)) notFound();
  // Mounted outside the organization gate so the code and state leave the
  // address bar even when the organization cannot be loaded.
  return <IntegrationOAuthCallback provider={provider} />;
}
