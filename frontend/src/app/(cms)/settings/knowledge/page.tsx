"use client";

import { Suspense } from "react";

import { KnowledgePage } from "@/features/knowledge/components/knowledge-page";

export default function KnowledgeRoute() {
  return (
    <Suspense>
      <KnowledgePage />
    </Suspense>
  );
}
