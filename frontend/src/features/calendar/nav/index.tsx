"use client";

// Ported from otopoly-go frontend/src/features/calendar/nav: today's count
// badge and info popover on the sidebar item.

import { useQuery } from "@tanstack/react-query";
import { format, isSameDay, parseISO } from "date-fns";

import { permissions } from "@/config/permissions";
import { calendarQueryKeys } from "@/features/calendar/hooks/use-calendar";
import { appointmentsService } from "@/features/calendar/services/calendar.service";
import { createNavAdornment, type NavAdornment } from "@/features/nav-engine";
import { useOptionalTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

function useCalendarNavAdornment(): NavAdornment {
  const { t } = useLocale();
  const tenant = useOptionalTenant();
  const slug = tenant?.slug ?? "";
  const { hasPermission } = usePermission();
  const canRead = hasPermission(permissions.tenant.appointments.read);
  const { data } = useQuery({
    queryKey: calendarQueryKeys.upcoming(slug, "mine"),
    queryFn: () =>
      appointmentsService.upcoming(slug, { scope: "mine", limit: 8 }),
    enabled: canRead && Boolean(slug),
    staleTime: 60_000,
    retry: false,
  });
  if (!data) return {};
  const today = new Date();
  const todays = data.items.filter((a) =>
    isSameDay(parseISO(a.starts_at), today),
  );
  return {
    badges:
      todays.length > 0
        ? [{ kind: "count", value: todays.length, variant: "secondary" }]
        : [],
    info: {
      title: t("calendar.nav.today"),
      rows:
        todays.length > 0
          ? todays.slice(0, 5).map((a) => ({
              label: a.title,
              value: format(parseISO(a.starts_at), "HH:mm"),
            }))
          : [{ label: t("calendar.nav.nothing_today"), value: 0 }],
    },
  };
}

export const CalendarNavAdornment = createNavAdornment(useCalendarNavAdornment);
