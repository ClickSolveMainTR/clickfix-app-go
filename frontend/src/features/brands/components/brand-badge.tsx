"use client";

import { cn } from "@/lib/utils";

import { useBrands } from "../hooks/use-brands";

/**
 * Compact brand chip (colour dot + name) for inbox rows and headers. Renders
 * nothing for brandless records or brands the viewer cannot read.
 */
export function BrandBadge({
  brandId,
  className,
}: {
  brandId: string | null | undefined;
  className?: string;
}) {
  const brands = useBrands();
  if (!brandId) return null;
  const brand = brands.data?.find((b) => b.uuid === brandId);
  if (!brand) return null;
  return (
    <span
      className={cn(
        "border-border text-muted-foreground inline-flex max-w-32 shrink-0 items-center gap-1 rounded-full border px-1.5 py-px text-[10px] leading-4 font-medium",
        className,
      )}
      title={brand.name}
    >
      <span
        aria-hidden
        className="size-2 shrink-0 rounded-full ring-1 ring-black/10 dark:ring-white/20"
        style={{ backgroundColor: brand.primary_color ?? "#2563eb" }}
      />
      <span className="truncate">{brand.name}</span>
    </span>
  );
}
