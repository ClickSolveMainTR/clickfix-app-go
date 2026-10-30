"use client";

import { addDays, format, parseISO } from "date-fns";

import { Button } from "@/components/ui/button";
import { DatePicker } from "@/components/ui/date-picker";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { useAppointmentSlots } from "@/features/calendar/hooks/use-calendar";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

/**
 * Day picker plus the free slots of an appointment type on that day (agent
 * view: shifts / business hours, time off and other bookings already
 * excluded by the API).
 */
export function SlotPicker({
  typeUuid,
  duration,
  hostUuid,
  day,
  onDayChange,
  value,
  onChange,
}: {
  typeUuid: string;
  duration: number;
  hostUuid: string;
  /** yyyy-MM-dd */
  day: string;
  onDayChange: (day: string) => void;
  /** Selected slot start (ISO) */
  value: string;
  onChange: (startsAt: string) => void;
}) {
  const { t } = useLocale();
  const to = day ? format(addDays(parseISO(day), 1), "yyyy-MM-dd") : "";
  const slots = useAppointmentSlots(typeUuid, day, to, duration, hostUuid);
  const items = slots.data?.items ?? [];

  return (
    <div className="space-y-2">
      <Label htmlFor="appointment-day">{t("appointments.book.day")}</Label>
      <DatePicker id="appointment-day" value={day} onChange={onDayChange} />
      <div className="space-y-1.5">
        <p className="text-muted-foreground text-xs">
          {t("appointments.book.slots")}
        </p>
        {!typeUuid ? (
          <p className="text-muted-foreground text-sm">
            {t("appointments.book.pick_type")}
          </p>
        ) : slots.isLoading ? (
          <div className="grid grid-cols-4 gap-1.5">
            {[0, 1, 2, 3].map((i) => (
              <Skeleton key={i} className="h-8" />
            ))}
          </div>
        ) : items.length === 0 ? (
          <p className="text-muted-foreground text-sm">
            {t("appointments.book.no_slots")}
          </p>
        ) : (
          <div
            role="listbox"
            aria-label={t("appointments.book.slots")}
            className="grid max-h-48 grid-cols-3 gap-1.5 overflow-y-auto sm:grid-cols-4"
          >
            {items.map((slot) => {
              const selected = slot.starts_at === value;
              return (
                <Button
                  key={slot.starts_at}
                  type="button"
                  role="option"
                  aria-selected={selected}
                  size="sm"
                  variant={selected ? "default" : "outline"}
                  className={cn("tabular-nums")}
                  onClick={() => onChange(slot.starts_at)}
                >
                  {format(parseISO(slot.starts_at), "HH:mm")}
                </Button>
              );
            })}
          </div>
        )}
      </div>
    </div>
  );
}
