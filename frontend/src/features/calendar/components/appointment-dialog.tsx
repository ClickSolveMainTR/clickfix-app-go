"use client";

import { format, parseISO } from "date-fns";
import {
  CalendarPlus,
  Copy,
  Download,
  ExternalLink,
  MessagesSquare,
} from "lucide-react";
import Link from "next/link";
import { useState } from "react";

import { StatusChip } from "@/components/common/status-chip";
import { FormActions } from "@/components/forms/form-actions";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { Textarea } from "@/components/ui/textarea";
import { permissions } from "@/config/permissions";
import { SlotPicker } from "@/features/calendar/components/slot-picker";
import { COLOR_CLASSES } from "@/features/calendar/components/views/types";
import { useNow } from "@/features/calendar/components/views/use-now";
import {
  useAppointment,
  useCalendarMutations,
} from "@/features/calendar/hooks/use-calendar";
import {
  LOCATION_KEYS,
  statusKey,
  statusTone,
} from "@/features/calendar/lib/layers";
import type { Appointment } from "@/features/calendar/types";
import { cn } from "@/lib/utils";
import { dateFnsLocale } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

type Mode = "view" | "reschedule" | "cancel";

/** Appointment detail with lifecycle actions, reschedule and links. */
export function AppointmentDialog({
  uuid,
  onOpenChange,
}: {
  uuid: string | null;
  onOpenChange: (open: boolean) => void;
}) {
  const query = useAppointment(uuid);
  return (
    <Dialog open={uuid !== null} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90dvh] overflow-y-auto sm:max-w-lg">
        {query.data ? (
          <AppointmentBody
            key={`${query.data.uuid}:${query.data.updated_at}`}
            appointment={query.data}
            onClose={() => onOpenChange(false)}
          />
        ) : (
          <div className="space-y-3">
            <DialogHeader>
              <DialogTitle>
                <Skeleton className="h-6 w-48" />
              </DialogTitle>
              <DialogDescription className="sr-only">…</DialogDescription>
            </DialogHeader>
            <Skeleton className="h-24 w-full" />
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}

function AppointmentBody({
  appointment: a,
  onClose,
}: {
  appointment: Appointment;
  onClose: () => void;
}) {
  const { t, locale } = useLocale();
  const dfLocale = dateFnsLocale(locale);
  const { hasPermission } = usePermission();
  const canWrite = hasPermission(permissions.tenant.appointments.write);
  const { setStatus, reschedule, updateNotes } = useCalendarMutations();
  const [mode, setMode] = useState<Mode>("view");
  const [reason, setReason] = useState("");
  const [notes, setNotes] = useState(a.notes);
  const start = parseISO(a.starts_at);
  const end = parseISO(a.ends_at);
  const [day, setDay] = useState(format(start, "yyyy-MM-dd"));
  const [newStart, setNewStart] = useState("");
  const active = a.status === "scheduled" || a.status === "confirmed";
  const now = useNow();
  const started = now !== null && start.getTime() <= now.getTime();
  const sKey = statusKey(a.status);
  const duration = Math.round((end.getTime() - start.getTime()) / 60_000);

  const changeStatus = (status: Appointment["status"], why = "") =>
    setStatus.mutate(
      { uuid: a.uuid, body: { status, reason: why } },
      {
        onSuccess: () => {
          appToast.success(t("appointments.detail.status_changed"));
          setMode("view");
        },
      },
    );

  const copyLink = async () => {
    try {
      await navigator.clipboard.writeText(a.links.manage);
      appToast.success(t("appointments.detail.link_copied"));
    } catch {
      appToast.error(t("appointments.detail.copy_failed"));
    }
  };

  return (
    <>
      <DialogHeader>
        <div className="flex items-center gap-2">
          <span
            aria-hidden
            className={cn(
              "size-2.5 rounded-full",
              COLOR_CLASSES[a.type.color as keyof typeof COLOR_CLASSES]?.dot ??
                "bg-primary",
            )}
          />
          <span className="text-muted-foreground text-xs font-medium tracking-wide uppercase">
            {a.type.name}
          </span>
          {sKey ? (
            <StatusChip label={t(sKey)} tone={statusTone(a.status)} />
          ) : null}
        </div>
        <DialogTitle
          className={cn(a.status === "cancelled" && "line-through opacity-70")}
        >
          {a.title}
        </DialogTitle>
        <DialogDescription className="first-letter:uppercase">
          {format(start, "PPPP · HH:mm", { locale: dfLocale })} –{" "}
          {format(end, "HH:mm")} ·{" "}
          {t("appointments.minutes", { count: duration })}
        </DialogDescription>
      </DialogHeader>

      {mode === "view" ? (
        <dl className="grid grid-cols-2 gap-x-4 gap-y-3 text-sm">
          <div>
            <dt className="text-muted-foreground text-xs">
              {t("appointments.detail.host")}
            </dt>
            <dd>{a.host?.name || "—"}</dd>
          </div>
          <div>
            <dt className="text-muted-foreground text-xs">
              {t("appointments.detail.location")}
            </dt>
            <dd className="break-words">
              {t(LOCATION_KEYS[a.location_kind])}
              {a.location_value ? (
                a.location_value.startsWith("https://") ? (
                  <a
                    href={a.location_value}
                    target="_blank"
                    rel="noreferrer"
                    className="text-primary block truncate hover:underline"
                  >
                    {a.location_value}
                  </a>
                ) : (
                  <span className="text-muted-foreground block">
                    {a.location_value}
                  </span>
                )
              ) : null}
            </dd>
          </div>
          <div className="col-span-2">
            <dt className="text-muted-foreground text-xs">
              {t("appointments.detail.invitee")}
            </dt>
            <dd>
              {a.invitee.name || "—"}
              {a.invitee.email ? (
                <span className="text-muted-foreground">
                  {" "}
                  · {a.invitee.email}
                </span>
              ) : null}
              {a.invitee.phone ? (
                <span className="text-muted-foreground">
                  {" "}
                  · {a.invitee.phone}
                </span>
              ) : null}
            </dd>
          </div>
          {a.contact || a.company || a.conversation ? (
            <div className="col-span-2 flex flex-wrap gap-2">
              {a.conversation ? (
                <Button asChild size="sm" variant="outline">
                  <Link href={`/inbox?c=${a.conversation.uuid}`}>
                    <MessagesSquare aria-hidden />#{a.conversation.number}
                  </Link>
                </Button>
              ) : null}
              {a.company ? (
                <Button asChild size="sm" variant="outline">
                  <Link href={`/companies/${a.company.uuid}`}>
                    {a.company.name}
                  </Link>
                </Button>
              ) : null}
              {a.contact && !a.company ? (
                <span className="text-muted-foreground self-center text-xs">
                  {a.contact.name}
                </span>
              ) : null}
            </div>
          ) : null}
          {Object.keys(a.intake_values).length > 0 ? (
            <div className="col-span-2">
              <dt className="text-muted-foreground text-xs">
                {t("appointments.detail.intake")}
              </dt>
              <dd className="mt-1 flex flex-wrap gap-1">
                {Object.entries(a.intake_values).map(([k, v]) => (
                  <span
                    key={k}
                    className="bg-muted rounded px-1.5 py-0.5 text-[11px]"
                  >
                    {k}: {typeof v === "object" ? JSON.stringify(v) : String(v)}
                  </span>
                ))}
              </dd>
            </div>
          ) : null}
          {a.status === "cancelled" && a.cancel_reason ? (
            <div className="col-span-2">
              <dt className="text-muted-foreground text-xs">
                {t("appointments.detail.cancel_reason")}
              </dt>
              <dd>{a.cancel_reason}</dd>
            </div>
          ) : null}
          <div className="col-span-2 space-y-1.5">
            <Label htmlFor="appointment-detail-notes" className="text-xs">
              {t("appointments.book.notes")}
            </Label>
            <Textarea
              id="appointment-detail-notes"
              value={notes}
              rows={3}
              maxLength={5000}
              disabled={!canWrite}
              onChange={(e) => setNotes(e.target.value)}
              onBlur={() => {
                if (canWrite && notes !== a.notes) {
                  updateNotes.mutate({ uuid: a.uuid, notes });
                }
              }}
            />
          </div>
          {a.status !== "cancelled" ? (
            <div className="col-span-2 flex flex-wrap gap-2">
              <Button asChild size="sm" variant="ghost">
                <a href={a.links.ics}>
                  <Download aria-hidden />
                  {t("appointments.detail.ics")}
                </a>
              </Button>
              {a.links.google ? (
                <Button asChild size="sm" variant="ghost">
                  <a href={a.links.google} target="_blank" rel="noreferrer">
                    <CalendarPlus aria-hidden />
                    {t("appointments.detail.google")}
                  </a>
                </Button>
              ) : null}
              <Button
                type="button"
                size="sm"
                variant="ghost"
                onClick={() => void copyLink()}
              >
                <Copy aria-hidden />
                {t("appointments.detail.copy_link")}
              </Button>
              <Button asChild size="sm" variant="ghost">
                <a href={a.links.manage} target="_blank" rel="noreferrer">
                  <ExternalLink aria-hidden />
                  {t("appointments.detail.manage_page")}
                </a>
              </Button>
            </div>
          ) : null}
        </dl>
      ) : mode === "reschedule" ? (
        <SlotPicker
          typeUuid={a.type.uuid}
          duration={duration}
          hostUuid={a.host?.uuid ?? ""}
          day={day}
          onDayChange={(d) => {
            setDay(d);
            setNewStart("");
          }}
          value={newStart}
          onChange={setNewStart}
        />
      ) : (
        <div className="space-y-1.5">
          <Label htmlFor="appointment-cancel-reason">
            {t("appointments.detail.cancel_reason")}
          </Label>
          <Textarea
            id="appointment-cancel-reason"
            value={reason}
            rows={3}
            maxLength={1000}
            onChange={(e) => setReason(e.target.value)}
          />
        </div>
      )}

      <FormActions>
        {mode === "view" ? (
          <>
            {canWrite && active ? (
              <>
                <Button
                  type="button"
                  variant="ghost"
                  className="text-destructive mr-auto"
                  onClick={() => setMode("cancel")}
                >
                  {t("appointments.detail.cancel")}
                </Button>
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => setMode("reschedule")}
                >
                  {t("appointments.detail.reschedule")}
                </Button>
                {started ? (
                  <>
                    <Button
                      type="button"
                      variant="outline"
                      disabled={setStatus.isPending}
                      onClick={() => changeStatus("no_show")}
                    >
                      {t("appointments.detail.no_show")}
                    </Button>
                    <Button
                      type="button"
                      disabled={setStatus.isPending}
                      onClick={() => changeStatus("completed")}
                    >
                      {t("appointments.detail.complete")}
                    </Button>
                  </>
                ) : a.status === "scheduled" ? (
                  <Button
                    type="button"
                    disabled={setStatus.isPending}
                    onClick={() => changeStatus("confirmed")}
                  >
                    {t("appointments.detail.confirm")}
                  </Button>
                ) : null}
              </>
            ) : (
              <Button type="button" variant="outline" onClick={onClose}>
                {t("common.close")}
              </Button>
            )}
          </>
        ) : (
          <>
            <Button
              type="button"
              variant="outline"
              onClick={() => setMode("view")}
            >
              {t("common.back")}
            </Button>
            {mode === "reschedule" ? (
              <Button
                type="button"
                disabled={!newStart || reschedule.isPending}
                onClick={() =>
                  reschedule.mutate(
                    { uuid: a.uuid, body: { starts_at: newStart } },
                    {
                      onSuccess: () => {
                        appToast.success(t("appointments.detail.rescheduled"));
                        setMode("view");
                      },
                    },
                  )
                }
              >
                {t("appointments.detail.reschedule")}
              </Button>
            ) : (
              <Button
                type="button"
                variant="destructive"
                disabled={setStatus.isPending}
                onClick={() => changeStatus("cancelled", reason.trim())}
              >
                {t("appointments.detail.cancel_confirm")}
              </Button>
            )}
          </>
        )}
      </FormActions>
    </>
  );
}
