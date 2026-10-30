import { ApiError } from "@/lib/api";

/**
 * Billing error codes the API answers with a specific status (CF-154). Each
 * has a `billing.errors.<CODE>` message in both locales.
 */
export const BILLING_ERROR_CODES = [
  "PAYMENT_GATEWAY_NOT_CONFIGURED",
  "PAYMENT_PROVIDER_DISABLED",
  "PAYMENT_PROVIDER_UNAVAILABLE",
  "PAYMENT_PROVIDER_REJECTED",
  "PAYMENT_VERIFICATION_FAILED",
  "PAYMENT_METHOD_UNAVAILABLE",
  "BILLING_SELLER_NOT_CONFIGURED",
  "BILLING_PROFILE_INCOMPLETE",
  "BILLING_ACCOUNT_MISSING",
  "CREDIT_PACKAGE_UNAVAILABLE",
  "CHECKOUT_IN_PROGRESS",
  "CHECKOUT_FINISHED",
  "INVOICE_NOT_PAYABLE",
] as const;

export type BillingErrorCode = (typeof BILLING_ERROR_CODES)[number];

export function isBillingErrorCode(code: unknown): code is BillingErrorCode {
  return (
    typeof code === "string" &&
    (BILLING_ERROR_CODES as readonly string[]).includes(code)
  );
}

/** Localized text of a billing failure; `fallbackKey` for anything else. */
export function billingErrorText(
  t: (key: string) => string,
  error: unknown,
  fallbackKey: string,
): string {
  if (error instanceof ApiError && isBillingErrorCode(error.code)) {
    return t(`billing.errors.${error.code}`);
  }
  return t(fallbackKey);
}

/** Text for a `reason` of the payment availability endpoint. */
export function billingReasonText(
  t: (key: string) => string,
  reason: string | undefined,
  fallbackKey: string,
): string {
  return isBillingErrorCode(reason)
    ? t(`billing.errors.${reason}`)
    : t(fallbackKey);
}
