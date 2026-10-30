"use client";

import { BrandLogo, isBrandSlug } from "@/components/brand-logos";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

/**
 * Payment provider as its original wordmark (PayTR, iyzico); havale/EFT and
 * the test provider stay plain text.
 */
export function PaymentProviderLabel({
  provider,
  className,
}: {
  provider: string;
  className?: string;
}) {
  const { t } = useLocale();
  const label = t(`billing.provider.${provider}`);
  if (isBrandSlug(provider)) {
    return (
      <BrandLogo
        slug={provider}
        title={label}
        className={cn("h-3.5 w-auto shrink-0", className)}
      />
    );
  }
  return <span className={className}>{label}</span>;
}
