import type { components } from "@/generated/api";

type S = components["schemas"];

export type BillingPlan = S["BillingPlan"];
export type BillingPlanInput = S["BillingPlanInput"];
export type BillingVersion = S["BillingVersion"];
export type BillingVersionInput = S["BillingVersionInput"];
export type BillingPrice = S["BillingPrice"];
export type BillingGrant = S["BillingGrant"];
export type BillingFeature = S["BillingFeature"];
export type BillingSubscription = S["BillingSubscription"];
export type BillingSubscriptionStatus = BillingSubscription["status"];
export type BillingTransition = S["BillingTransition"];
export type BillingHistoryEntry = S["BillingHistory"];
export type BillingOverride = S["BillingOverride"];
export type BillingUsageSummary = S["BillingUsageSummary"];
export type EntitlementSnapshot = S["EntitlementSnapshot"];
export type EntitlementValue = S["EntitlementValue"];
export type BillingBankAccount = S["BillingBankAccount"];
export type BillingBankAccountInput = S["BillingBankAccountInput"];
export type BillingInvoiceSummary = S["BillingInvoiceSummary"];
export type BillingPaymentCard = S["BillingPaymentCard"];
export type BillingTransferSummary = S["BillingTransferSummary"];
export type BillingTransferInput = S["BillingTransferInput"];
export type BillingCheckoutInput = S["BillingCheckoutInput"];
export type BillingCheckoutOutput = S["BillingCheckoutOutput"];
export type BillingAdminSubscription = S["BillingAdminSubscription"];
export type BillingAdminSubscriptionStats = S["BillingAdminSubscriptionStats"];
export type BillingAdminPayment = S["BillingAdminPayment"];
export type BillingAdminTransfer = S["BillingAdminTransfer"];
export type BillingWalletSummary = S["BillingWalletSummary"];
export type BillingWalletEntry = S["BillingWalletEntry"];
export type BillingWalletEntryInput = S["BillingWalletEntryInput"];
export type BillingWalletUnit = BillingWalletEntryInput["unit"];

export type Page<T> = {
  items: T[];
  total: number;
  limit: number;
  offset: number;
};

export type AdminListParams = {
  limit: number;
  offset: number;
  q?: string;
  status?: string;
  live?: "true";
};
