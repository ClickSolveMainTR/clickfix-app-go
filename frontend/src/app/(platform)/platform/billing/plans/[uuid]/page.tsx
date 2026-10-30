import { PlanEditorPage } from "@/features/billing";

export default async function PlatformBillingPlanPage({
  params,
}: {
  params: Promise<{ uuid: string }>;
}) {
  const { uuid } = await params;
  return <PlanEditorPage uuid={uuid} />;
}
