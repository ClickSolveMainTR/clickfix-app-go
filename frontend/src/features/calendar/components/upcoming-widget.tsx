"use client";

// Ported from otopoly-go frontend/src/features/calendar/components/upcoming-widget.tsx:
// dashboard card of the next appointments, grouped by day.

import { addDays, format, isSameDay, parseISO } from "date-fns";
import { CalendarDays } from "lucide-react";
import Link from "next/link";
import { useMemo } from "react";

import { Skeleton } from "@/components/ui/skeleton";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { COLOR_CLASSES } from "@/features/calendar/components/views/types";
import { useUpcomingAppointments } from "@/features/calendar/hooks/use-calendar";
import type { Appointment } from "@/features/calendar/types";
import { cn } from "@/lib/utils";
import { dateFnsLocale } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

/** Dashboard card: my next appointments. */
export function AppointmentsUpcomingWidget() {
  const { t, locale } = useLocale();
  const dfLocale = dateFnsLocale(locale);
  const { hasPermission } = usePermission();
  const canRead = hasPermission(permissions.tenant.appointments.read);
  const upcoming = useUpcomingAppointments("mine", canRead);
  const today = useMemo(() => new Date(), []);

  const groups = useMemo(() => {
    const out: { day: Date; items: Appointment[] }[] = [];
    for (const item of upcoming.data?.items ?? []) {
      const day = parseISO(item.starts_at);
      const last = out[out.length - 1];
      if (last && isSameDay(last.day, day)) last.items.push(item);
      else out.push({ day, items: [item] });
    }
    return out;
  }, [upcoming.data]);

  // Hidden when the module is off for the organization (403).
  if (!canRead || upcoming.isError) return null;

  const dayLabel = (day: Date) => {
    if (isSameDay(day, today)) return t("calendar.today");
    if (isSameDay(day, addDays(today, 1))) return t("calendar.tomorrow");
    return format(day, "EEEE, d MMMM", { locale: dfLocale });
  };

  return (
    <section
      className="bg-card flex flex-col rounded-2xl border"
      data-widget="appointments"
    >
      <div className="flex items-center justify-between gap-2 border-b px-4 py-3">
        <h2 className="flex items-center gap-2 text-sm font-semibold">
          <CalendarDays className="text-primary size-4" />
          {t("calendar.widget.title")}
        </h2>
        <Link
          href={routes.cms.calendar.root}
          className="text-primary text-xs font-medium hover:underline"
        >
          {t("calendar.widget.open")}
        </Link>
      </div>
      {upcoming.isLoading ? (
        <div className="space-y-2 p-4">
          {[0, 1, 2].map((i) => (
            <Skeleton key={i} className="h-8 w-full" />
          ))}
        </div>
      ) : groups.length === 0 ? (
        <p className="text-muted-foreground px-6 py-6 text-center text-sm">
          {t("calendar.widget.empty")}
        </p>
      ) : (
        <div className="divide-y">
          {groups.map((group) => (
            <div key={group.day.toISOString()} className="py-1">
              <p className="text-muted-foreground px-4 pt-1.5 pb-0.5 text-[11px] font-medium tracking-wide uppercase">
                {dayLabel(group.day)}
              </p>
              <ul>
                {group.items.map((item) => (
                  <li key={item.uuid}>
                    <Link
                      href={routes.cms.calendar.appointment(item.uuid)}
                      className="hover:bg-muted/40 flex items-center gap-3 px-4 py-2 transition-colors"
                    >
                      <span
                        aria-hidden
                        className={cn(
                          "size-2 shrink-0 rounded-full",
                          COLOR_CLASSES[
                            item.type.color as keyof typeof COLOR_CLASSES
                          ]?.dot ?? "bg-primary",
                        )}
                      />
                      <span className="min-w-0 flex-1">
                        <span className="block truncate text-sm font-medium">
                          {item.title}
                        </span>
                        <span className="text-muted-foreground block truncate text-xs">
                          {item.type.name}
                          {item.invitee.name ? ` · ${item.invitee.name}` : ""}
                        </span>
                      </span>
                      <span className="text-muted-foreground shrink-0 text-xs tabular-nums">
                        {format(parseISO(item.starts_at), "HH:mm")}
                      </span>
                    </Link>
                  </li>
                ))}
              </ul>
            </div>
          ))}
        </div>
      )}
    </section>
  );
}
