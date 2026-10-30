"use client";

import { DatePicker } from "@/components/ui/date-picker";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { CHANNEL_TYPES } from "@/features/sla/lib/format";
import type { DirectoryOption } from "@/features/tenant-members/hooks/use-member-directory";
import { useLocale } from "@/providers/locale-provider";

export const ANY = "__any__";

/** Inclusive day range (`yyyy-MM-dd`). */
export function DateRangeFields({
  from,
  to,
  onChange,
}: {
  from: string;
  to: string;
  onChange: (range: { from: string; to: string }) => void;
}) {
  const { t } = useLocale();
  return (
    <>
      <div className="grid gap-1.5">
        <Label className="text-xs">{t("reports.filters.from")}</Label>
        <DatePicker
          value={from}
          onChange={(v) => onChange({ from: v, to })}
          className="w-full sm:w-40"
        />
      </div>
      <div className="grid gap-1.5">
        <Label className="text-xs">{t("reports.filters.to")}</Label>
        <DatePicker
          value={to}
          onChange={(v) => onChange({ from, to: v })}
          className="w-full sm:w-40"
        />
      </div>
    </>
  );
}

/** Select with an "any" entry mapped to `""`. */
export function OptionalSelect({
  id,
  label,
  value,
  options,
  anyLabel,
  onChange,
  className,
}: {
  id: string;
  label: string;
  value: string;
  options: DirectoryOption[];
  anyLabel: string;
  onChange: (value: string) => void;
  className?: string;
}) {
  return (
    <div className="grid gap-1.5">
      <Label htmlFor={id} className="text-xs">
        {label}
      </Label>
      <Select
        value={value || ANY}
        onValueChange={(v) => onChange(v === ANY ? "" : v)}
      >
        <SelectTrigger id={id} className={className ?? "w-full sm:w-44"}>
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={ANY}>{anyLabel}</SelectItem>
          {options.map((o) => (
            <SelectItem key={o.value} value={o.value}>
              {o.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}

export function useChannelOptions(): DirectoryOption[] {
  const { t } = useLocale();
  return CHANNEL_TYPES.map((c) => ({ value: c, label: t(`sla.channel.${c}`) }));
}
