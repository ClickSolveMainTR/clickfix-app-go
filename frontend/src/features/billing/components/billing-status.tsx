"use client";

import { Badge } from "@/components/ui/badge";
import { useLocale } from "@/providers/locale-provider";

type Tone = "secondary" | "success" | "warning" | "danger" | "outline";

const SUBSCRIPTION_TONES: Record<string, Tone> = {
  pending_payment: "warning",
  trialing: "secondary",
  active: "success",
  past_due: "warning",
  grace: "warning",
  suspended: "danger",
  paused: "outline",
  canceled: "outline",
  expired: "danger",
};

const PAYMENT_TONES: Record<string, Tone> = {
  pending: "secondary",
  unknown: "warning",
  succeeded: "success",
  failed: "danger",
  expired: "outline",
};

const INVOICE_TONES: Record<string, Tone> = {
  draft: "outline",
  open: "warning",
  paid: "success",
  void: "outline",
  uncollectible: "danger",
};

const TRANSFER_TONES: Record<string, Tone> = {
  reported: "warning",
  approved: "success",
  rejected: "danger",
};

const MAPS = {
  subscription: SUBSCRIPTION_TONES,
  payment: PAYMENT_TONES,
  invoice: INVOICE_TONES,
  transfer: TRANSFER_TONES,
} as const;

/** Status pill for subscriptions, payments, invoices and bank transfers. */
export function BillingStatus({
  kind,
  status,
}: {
  kind: keyof typeof MAPS;
  status: string;
}) {
  const { t } = useLocale();
  const key = `billing.status.${kind}.${status}`;
  const label = t(key);
  return (
    <Badge variant={MAPS[kind][status] ?? "secondary"}>
      {label === key ? status : label}
    </Badge>
  );
}
