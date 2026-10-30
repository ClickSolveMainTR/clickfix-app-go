"use client";

import { notFound, useParams } from "next/navigation";

import { OAuthCallback } from "@/features/channels";
import { ActiveOrganizationGate } from "@/features/tenant";

export default function Page() {
  const { type } = useParams<{ type: string }>();
  if (type !== "instagram" && type !== "messenger" && type !== "slack") {
    notFound();
  }
  return (
    <ActiveOrganizationGate>
      <OAuthCallback type={type} />
    </ActiveOrganizationGate>
  );
}
