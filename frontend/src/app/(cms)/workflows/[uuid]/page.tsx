"use client";

import { useParams } from "next/navigation";

import { WorkflowEditorPage } from "@/features/workflows/components/workflow-editor";
import { ActiveOrganizationGate } from "@/features/tenant";

/** Canvas editor of one workflow (CF-96). */
export default function Page() {
  const { uuid } = useParams<{ uuid: string }>();
  return (
    <ActiveOrganizationGate>
      <WorkflowEditorPage uuid={uuid} />
    </ActiveOrganizationGate>
  );
}
