"use client";

import { ExternalLink, ReceiptText } from "lucide-react";
import Link from "next/link";

import { EmptyState } from "@/components/common/empty-state";
import { Button } from "@/components/ui/button";
import { routes } from "@/config/routes";
import { useLocale } from "@/providers/locale-provider";

/**
 * "Faturalar": the platform API has no per-organization invoice list yet,
 * so the tab explains where invoices and payments are followed (the billing
 * page) instead of showing an empty table.
 */
export function OrganizationInvoicesTab() {
  const { t } = useLocale();
  return (
    <EmptyState
      icon={<ReceiptText className="text-signal size-5" />}
      title={t("commerce.org.invoices.title")}
      description={t("commerce.org.invoices.description")}
      action={
        <Button asChild variant="outline">
          <Link href={routes.platform.billing.tab("payments")}>
            <ExternalLink className="size-4" />
            {t("commerce.org.invoices.open_billing")}
          </Link>
        </Button>
      }
    />
  );
}
