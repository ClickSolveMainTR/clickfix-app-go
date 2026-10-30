"use client";

import { useState } from "react";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { useLocale } from "@/providers/locale-provider";

import { TIMEZONES, WEEK_DAYS } from "../lib/format";
import type {
  SlaBusinessHours,
  SlaBusinessHoursInput,
  SlaWeek,
} from "../services/sla.service";

const DEFAULT_WEEK: SlaWeek = {
  mon: { closed: false, open: "09:00", close: "18:00" },
  tue: { closed: false, open: "09:00", close: "18:00" },
  wed: { closed: false, open: "09:00", close: "18:00" },
  thu: { closed: false, open: "09:00", close: "18:00" },
  fri: { closed: false, open: "09:00", close: "18:00" },
  sat: { closed: true, open: "", close: "" },
  sun: { closed: true, open: "", close: "" },
};

/** Create or edit a weekly business hours schedule. */
export function HoursDialog({
  open,
  onOpenChange,
  hours,
  pending,
  onSubmit,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  hours: SlaBusinessHours | null;
  pending: boolean;
  onSubmit: (body: SlaBusinessHoursInput) => void;
}) {
  const { t } = useLocale();
  const [name, setName] = useState(hours?.name ?? "");
  const [timezone, setTimezone] = useState(
    hours?.timezone ?? "Europe/Istanbul",
  );
  const [holidays, setHolidays] = useState(hours?.use_public_holidays ?? true);
  const [isDefault, setIsDefault] = useState(hours?.is_default ?? false);
  const [week, setWeek] = useState<SlaWeek>(() => {
    const base = { ...DEFAULT_WEEK };
    if (hours) {
      for (const d of WEEK_DAYS) {
        base[d] = hours.working_hours[d] ?? {
          closed: true,
          open: "",
          close: "",
        };
      }
    }
    return base;
  });

  const setDay = (day: string, patch: Partial<SlaWeek[string]>) =>
    setWeek((w) => {
      const cur = w[day] ?? { closed: true, open: "", close: "" };
      const next = { ...cur, ...patch };
      if (!next.closed && !next.open) {
        next.open = "09:00";
        next.close = "18:00";
      }
      return { ...w, [day]: next };
    });

  const zones: string[] = TIMEZONES.includes(
    timezone as (typeof TIMEZONES)[number],
  )
    ? [...TIMEZONES]
    : [timezone, ...TIMEZONES];

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>
            {hours ? t("sla.hours.edit") : t("sla.hours.new")}
          </DialogTitle>
          <DialogDescription>{t("sla.hours.dialog_hint")}</DialogDescription>
        </DialogHeader>
        <div className="grid gap-4">
          <div className="grid gap-1.5">
            <Label htmlFor="bh-name">{t("sla.fields.name")}</Label>
            <Input
              id="bh-name"
              value={name}
              maxLength={120}
              onChange={(e) => setName(e.target.value)}
            />
          </div>
          <div className="grid gap-1.5">
            <Label>{t("sla.fields.timezone")}</Label>
            <Select value={timezone} onValueChange={setTimezone}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {zones.map((z) => (
                  <SelectItem key={z} value={z}>
                    {z}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="grid gap-2">
            {WEEK_DAYS.map((d) => {
              const day = week[d] ?? { closed: true, open: "", close: "" };
              return (
                <div key={d} className="flex items-center gap-3">
                  <Switch
                    checked={!day.closed}
                    onCheckedChange={(on) => setDay(d, { closed: !on })}
                    aria-label={t(`sla.day.${d}`)}
                  />
                  <span className="w-24 text-sm">{t(`sla.day.${d}`)}</span>
                  {day.closed ? (
                    <span className="text-muted-foreground text-sm">
                      {t("sla.hours.closed")}
                    </span>
                  ) : (
                    <div className="flex items-center gap-2">
                      <Input
                        type="time"
                        className="w-28"
                        value={day.open}
                        aria-label={`${t(`sla.day.${d}`)} ${t("sla.hours.open")}`}
                        onChange={(e) => setDay(d, { open: e.target.value })}
                      />
                      <span className="text-muted-foreground">–</span>
                      <Input
                        type="time"
                        className="w-28"
                        value={day.close}
                        aria-label={`${t(`sla.day.${d}`)} ${t("sla.hours.close")}`}
                        onChange={(e) => setDay(d, { close: e.target.value })}
                      />
                    </div>
                  )}
                </div>
              );
            })}
          </div>
          <div className="flex items-center gap-2">
            <Switch
              id="bh-holidays"
              checked={holidays}
              onCheckedChange={setHolidays}
            />
            <Label htmlFor="bh-holidays">
              {t("sla.fields.use_public_holidays")}
            </Label>
          </div>
          <div className="flex items-center gap-2">
            <Switch
              id="bh-default"
              checked={isDefault}
              disabled={hours?.is_default}
              onCheckedChange={setIsDefault}
            />
            <Label htmlFor="bh-default">{t("sla.fields.is_default")}</Label>
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t("common.cancel")}
          </Button>
          <Button
            disabled={pending || !name.trim()}
            onClick={() =>
              onSubmit({
                name: name.trim(),
                timezone,
                working_hours: week,
                use_public_holidays: holidays,
                holiday_country: "TR",
                team_id: hours?.team_id ?? null,
                is_default: isDefault,
              })
            }
          >
            {t("common.save")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
