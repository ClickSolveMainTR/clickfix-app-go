"use client";

import { Controller, get, useFormContext, useWatch } from "react-hook-form";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import {
  WEEK_DAY_LABEL_KEYS,
  WEEK_DAYS,
} from "@/features/tenant-settings/constants";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

type WeekHoursEditorProps = {
  /** Form path of the seven `{day, open, start, end}` rows, e.g. `business_hours.days`. */
  name: string;
  disabled?: boolean;
};

function DayRow({
  name,
  index,
  disabled,
}: {
  name: string;
  index: number;
  disabled?: boolean;
}) {
  const { t } = useLocale();
  const {
    control,
    register,
    formState: { errors },
  } = useFormContext();
  const day = WEEK_DAYS[index]!;
  const path = `${name}.${index}`;
  const open = useWatch({ name: `${path}.open` }) as boolean | undefined;
  const startError = get(errors, `${path}.start`)?.message as
    string | undefined;
  const endError = get(errors, `${path}.end`)?.message as string | undefined;
  const dayLabel = t(WEEK_DAY_LABEL_KEYS[day]);

  return (
    <div className="grid grid-cols-[minmax(0,1fr)_auto] items-start gap-3 border-b py-3 last:border-b-0 sm:grid-cols-[10rem_7rem_minmax(0,1fr)_minmax(0,1fr)] sm:items-center">
      <span className="text-sm font-medium">{dayLabel}</span>
      <Controller
        control={control}
        name={`${path}.open`}
        render={({ field }) => (
          <label className="flex items-center gap-2 text-sm">
            <Switch
              checked={Boolean(field.value)}
              onCheckedChange={field.onChange}
              disabled={disabled}
              aria-label={dayLabel}
            />
            <span className="text-muted-foreground">
              {field.value
                ? t("tenant_settings.fields.day_open")
                : t("tenant_settings.fields.day_closed")}
            </span>
          </label>
        )}
      />
      <div className="col-span-2 grid grid-cols-2 gap-3 sm:col-span-2">
        <div className="space-y-1">
          <Input
            type="time"
            step={60}
            aria-label={`${dayLabel} · ${t("tenant_settings.fields.start")}`}
            aria-invalid={Boolean(startError)}
            disabled={disabled || !open}
            className={cn(!open && "opacity-50")}
            {...register(`${path}.start`)}
          />
          {startError ? (
            <p className="text-destructive text-xs">{startError}</p>
          ) : null}
        </div>
        <div className="space-y-1">
          <Input
            type="time"
            step={60}
            aria-label={`${dayLabel} · ${t("tenant_settings.fields.end")}`}
            aria-invalid={Boolean(endError)}
            disabled={disabled || !open}
            className={cn(!open && "opacity-50")}
            {...register(`${path}.end`)}
          />
          {endError ? (
            <p className="text-destructive text-xs">{endError}</p>
          ) : null}
        </div>
      </div>
    </div>
  );
}

/**
 * Seven weekday rows (open switch, start, end) bound to `name` in the
 * surrounding react-hook-form. Used by the organization business hours and
 * the team working hours.
 */
export function WeekHoursEditor({ name, disabled }: WeekHoursEditorProps) {
  const { t } = useLocale();
  const { getValues, setValue } = useFormContext();

  const copyMondayToWeekdays = () => {
    const monday = getValues(`${name}.0`) as Record<string, unknown>;
    for (let i = 1; i < 5; i++) {
      setValue(
        `${name}.${i}`,
        { ...monday, day: WEEK_DAYS[i]! },
        { shouldDirty: true, shouldValidate: true },
      );
    }
  };

  return (
    <div className="space-y-2">
      <div className="hidden grid-cols-[10rem_7rem_minmax(0,1fr)_minmax(0,1fr)] gap-3 text-xs font-medium sm:grid">
        <span />
        <span />
        <span className="text-muted-foreground">
          {t("tenant_settings.fields.start")}
        </span>
        <span className="text-muted-foreground">
          {t("tenant_settings.fields.end")}
        </span>
      </div>
      <div>
        {WEEK_DAYS.map((day, index) => (
          <DayRow key={day} name={name} index={index} disabled={disabled} />
        ))}
      </div>
      {disabled ? null : (
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={copyMondayToWeekdays}
        >
          {t("tenant_settings.hours.copy_to_weekdays")}
        </Button>
      )}
    </div>
  );
}
