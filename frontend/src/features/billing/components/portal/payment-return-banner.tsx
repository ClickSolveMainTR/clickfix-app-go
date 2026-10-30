"use client";

import { CheckCircle2, Clock, Loader2, XCircle } from "lucide-react";
import { useRouter, useSearchParams } from "next/navigation";
import { useState } from "react";

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { routes } from "@/config/routes";
import { useCheckoutStatus } from "@/features/billing/hooks/use-tenant-billing";
import { ApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";

type View =
  "processing" | "succeeded" | "failed" | "expired" | "cancelled" | "unknown";

/**
 * Result of a hosted checkout after the provider sends the browser back
 * (`/billing?payment=…&payment_ref=…`). The query string is only a hint: the
 * page polls the server-verified status and never shows "paid" on its own.
 */
export function PaymentReturnBanner() {
  const { t } = useLocale();
  const router = useRouter();
  const search = useSearchParams();
  const reference = search.get("payment_ref");
  const hint = search.get("payment");
  const status = useCheckoutStatus(reference);
  const [since] = useState(() => Date.now());
  if (!reference) return null;

  const server = status.data?.status;
  let view: View = "processing";
  if (server === "succeeded" || server === "failed" || server === "expired") {
    view = server;
  } else if (status.error instanceof ApiError && status.error.status === 404) {
    view = "unknown";
  } else if (hint === "failed" || hint === "error") {
    // The provider page reported a failure or the payer left it; nothing
    // was charged unless a verified result says otherwise later.
    view = hint === "failed" ? "cancelled" : "unknown";
  }
  // Still unconfirmed after two minutes: say it lands in the invoice list.
  const slow =
    view === "processing" &&
    (status.isError || status.dataUpdatedAt - since > 120_000);
  const dismiss = () => router.replace(routes.cms.billing);
  const Icon =
    view === "succeeded"
      ? CheckCircle2
      : view === "processing"
        ? Loader2
        : view === "expired"
          ? Clock
          : XCircle;

  return (
    <Alert
      variant={
        view === "failed" || view === "unknown" ? "destructive" : "default"
      }
      role="status"
      aria-live="polite"
      data-testid="payment-return-banner"
      data-state={view}
    >
      <Icon
        className={view === "processing" ? "animate-spin" : undefined}
        aria-hidden
      />
      <AlertTitle>{t(`billing.portal.return.${view}_title`)}</AlertTitle>
      <AlertDescription>
        <p>
          {t(
            slow
              ? "billing.portal.return.slow"
              : `billing.portal.return.${view}`,
          )}
        </p>
        {view !== "processing" ? (
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="mt-2"
            onClick={dismiss}
          >
            {t("billing.portal.return.dismiss")}
          </Button>
        ) : null}
      </AlertDescription>
    </Alert>
  );
}
