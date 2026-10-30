"use client";

import { useParams } from "next/navigation";

import { RuleEditorPage } from "@/features/automation-rules/components/rule-editor";
import { ActiveOrganizationGate } from "@/features/tenant";

/** Editor of one automation rule; `new` creates one (CF-94). */
export default function Page() {
  const { uuid } = useParams<{ uuid: string }>();
  return (
    <ActiveOrganizationGate>
      <RuleEditorPage uuid={uuid} />
    </ActiveOrganizationGate>
  );
}
