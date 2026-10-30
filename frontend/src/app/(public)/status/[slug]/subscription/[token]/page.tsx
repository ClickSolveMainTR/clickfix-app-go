import { StatusSubscriptionPage } from "@/features/status-subscription/components/status-subscription-page";

export default async function StatusSubscriptionRoute({
  params,
}: {
  params: Promise<{ slug: string; token: string }>;
}) {
  const { slug, token } = await params;
  return <StatusSubscriptionPage slug={slug} token={token} />;
}
