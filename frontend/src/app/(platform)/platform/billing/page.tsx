import { BillingAdminPage } from "@/features/billing";

export default async function PlatformBillingPage({
  searchParams,
}: {
  searchParams: Promise<{ tab?: string | string[] }>;
}) {
  const { tab } = await searchParams;
  return (
    <BillingAdminPage initialTab={typeof tab === "string" ? tab : undefined} />
  );
}
