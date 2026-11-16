import { notFound } from "next/navigation";

import { DevCustomerPage } from "@/features/remote-support/components/dev-customer-page";

/**
 * Development stand-in for the customer's desktop app (CF-237, until
 * CF-239). Production builds answer 404: the page never ships enabled.
 */
export default function DevRemoteSupportRoute() {
  if (process.env.NODE_ENV !== "development") notFound();
  return <DevCustomerPage />;
}
