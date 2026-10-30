"use client";

import { format, parseISO } from "date-fns";
import { CalendarPlus } from "lucide-react";
import Link from "next/link";
import { useState } from "react";

import { StatusChip } from "@/components/common/status-chip";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  BookDialog,
  type BookingContext,
} from "@/features/calendar/components/book-dialog";
import { useLinkedAppointments } from "@/features/calendar/hooks/use-calendar";
import { statusKey, statusTone } from "@/features/calendar/lib/layers";
import { dateFnsLocale } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

/**
 * Appointments of a conversation, contact or company with a "book" action
 * (agent books for the customer). Hidden when the module is off.
 */
export function LinkedAppointmentsCard({
  context,
  compact = false,
}: {
  context: BookingContext;
  /** Inbox side panel style (small heading, no card chrome). */
  compact?: boolean;
}) {
  const { t, locale } = useLocale();
  const dfLocale = dateFnsLocale(locale);
  const { hasPermission } = usePermission();
  const canRead = hasPermission(permissions.tenant.appointments.read);
  const canWrite = hasPermission(permissions.tenant.appointments.write);
  const [open, setOpen] = useState(false);
  const linked = useLinkedAppointments(
    {
      conversation: context.conversationUuid,
      contact: context.conversationUuid ? undefined : context.contactUuid,
      company:
        context.conversationUuid || context.contactUuid
          ? undefined
          : context.companyUuid,
    },
    canRead,
  );
  if (!canRead || linked.isError) return null;
  const items = linked.data?.items ?? [];

  return (
    <section
      className={compact ? undefined : "bg-card rounded-2xl border p-4"}
      data-section="appointments"
    >
      <div className="mb-2 flex items-center gap-1">
        <h3
          className={
            compact
              ? "text-muted-foreground text-[11px] font-semibold tracking-wide uppercase"
              : "text-sm font-semibold"
          }
        >
          {t("appointments.linked.title")}
        </h3>
        {canWrite ? (
          <Button
            variant="ghost"
            size="icon"
            className="ml-auto size-6"
            title={t("appointments.linked.book")}
            aria-label={t("appointments.linked.book")}
            onClick={() => setOpen(true)}
          >
            <CalendarPlus className="size-3.5" />
          </Button>
        ) : null}
      </div>
      {items.length === 0 ? (
        <p className="text-muted-foreground text-xs">
          {t("appointments.linked.empty")}
        </p>
      ) : (
        <ul className="flex flex-col gap-1">
          {items.slice(0, 5).map((a) => {
            const sKey = statusKey(a.status);
            return (
              <li key={a.uuid}>
                <Link
                  href={routes.cms.calendar.appointment(a.uuid)}
                  className="hover:bg-muted flex items-center gap-2 rounded-md px-2 py-1.5"
                >
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-xs font-medium">
                      {a.type.name}
                    </span>
                    <span className="text-muted-foreground block text-[11px] first-letter:uppercase">
                      {format(parseISO(a.starts_at), "PPp", {
                        locale: dfLocale,
                      })}
                    </span>
                  </span>
                  {sKey ? (
                    <StatusChip label={t(sKey)} tone={statusTone(a.status)} />
                  ) : null}
                </Link>
              </li>
            );
          })}
        </ul>
      )}
      {open ? (
        <BookDialog open onOpenChange={setOpen} context={context} />
      ) : null}
    </section>
  );
}
