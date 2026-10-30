"use client";

import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useLocale } from "@/providers/locale-provider";

import { useBrands } from "../hooks/use-brands";

const NONE = "__none__";

/**
 * Brand picker for channels, widgets and help center articles (CF-124).
 * Hidden when the organization has no brands (single-brand organizations
 * never see it). `allowNone` adds "No brand" (organization identity).
 */
export function BrandSelect({
  id,
  value,
  onChange,
  disabled,
  allowNone = true,
  label,
  hint,
  placeholder,
}: {
  id: string;
  value: string | null | undefined;
  onChange: (brandId: string | null) => void;
  disabled?: boolean;
  allowNone?: boolean;
  label?: string;
  hint?: string;
  placeholder?: string;
}) {
  const { t } = useLocale();
  const brands = useBrands();
  const items = brands.data ?? [];
  if (items.length === 0) return null;
  return (
    <div className="space-y-2">
      <Label htmlFor={id}>{label ?? t("brands.select.label")}</Label>
      <Select
        value={value ?? (allowNone ? NONE : "")}
        onValueChange={(v) => onChange(v === NONE ? null : v)}
        disabled={disabled}
      >
        <SelectTrigger id={id} className="w-full">
          <SelectValue placeholder={placeholder ?? t("brands.select.pick")} />
        </SelectTrigger>
        <SelectContent>
          {allowNone && (
            <SelectItem value={NONE}>{t("brands.select.none")}</SelectItem>
          )}
          {items.map((b) => (
            <SelectItem key={b.uuid} value={b.uuid}>
              <span
                aria-hidden
                className="size-2.5 rounded-full ring-1 ring-black/10 dark:ring-white/20"
                style={{ backgroundColor: b.primary_color ?? "#2563eb" }}
              />
              {b.name}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      {hint && <p className="text-muted-foreground text-xs">{hint}</p>}
    </div>
  );
}
