"use client";

import { format } from "date-fns";
import { useState } from "react";

import { FormActions } from "@/components/forms/form-actions";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
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
import { Textarea } from "@/components/ui/textarea";
import { SlotPicker } from "@/features/calendar/components/slot-picker";
import {
  useAppointmentTypes,
  useCalendarMutations,
} from "@/features/calendar/hooks/use-calendar";
import type { Appointment } from "@/features/calendar/types";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/** Where the booking starts from (conversation / contact / company). */
export type BookingContext = {
  conversationUuid?: string;
  contactUuid?: string;
  companyUuid?: string;
  /** Shown under the title, e.g. the contact's name. */
  label?: string;
  inviteeName?: string;
  inviteeEmail?: string;
};

const ANY_HOST = "__any";

/**
 * Agent books an appointment for a customer (ported from the otopoly-go
 * calendar event dialog). The slot list comes from the availability API.
 */
export function BookDialog({
  open,
  onOpenChange,
  draftDay,
  context,
  onBooked,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Pre-selected day (calendar slot click). */
  draftDay?: Date | null;
  context?: BookingContext;
  onBooked?: (appointment: Appointment) => void;
}) {
  const { t } = useLocale();
  const types = useAppointmentTypes(open);
  const { book } = useCalendarMutations();
  const active = (types.data?.items ?? []).filter((x) => x.is_active);

  const [typeUuid, setTypeUuid] = useState("");
  const [duration, setDuration] = useState(0);
  const [hostUuid, setHostUuid] = useState(ANY_HOST);
  const [day, setDay] = useState(() =>
    format(draftDay ?? new Date(), "yyyy-MM-dd"),
  );
  const [startsAt, setStartsAt] = useState("");
  const [name, setName] = useState(context?.inviteeName ?? "");
  const [email, setEmail] = useState(context?.inviteeEmail ?? "");
  const [phone, setPhone] = useState("");
  const [notes, setNotes] = useState("");
  const [notify, setNotify] = useState(true);

  const type = active.find((x) => x.uuid === typeUuid);
  const hasLink = Boolean(
    context?.contactUuid || context?.companyUuid || context?.conversationUuid,
  );
  const canSubmit =
    Boolean(type && startsAt) && (hasLink || name.trim().length > 0);

  const submit = () => {
    if (!type || !startsAt) return;
    book.mutate(
      {
        type_uuid: type.uuid,
        starts_at: startsAt,
        duration: duration || type.default_duration,
        host_uuid: hostUuid === ANY_HOST ? null : hostUuid,
        conversation_uuid: context?.conversationUuid ?? null,
        contact_uuid: context?.contactUuid ?? null,
        company_uuid: context?.companyUuid ?? null,
        invitee_name: name.trim(),
        invitee_email: email.trim(),
        invitee_phone: phone.trim(),
        notes: notes.trim(),
        notify,
      },
      {
        onSuccess: (appointment) => {
          appToast.success(t("appointments.book.done"));
          onBooked?.(appointment);
          onOpenChange(false);
        },
      },
    );
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90dvh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t("appointments.book.title")}</DialogTitle>
          <DialogDescription>
            {context?.label
              ? t("appointments.book.for", { name: context.label })
              : t("appointments.book.description")}
          </DialogDescription>
        </DialogHeader>
        <form
          className="space-y-4"
          onSubmit={(e) => {
            e.preventDefault();
            submit();
          }}
        >
          <div className="grid gap-3 sm:grid-cols-2">
            <div className="space-y-1.5 sm:col-span-2">
              <Label htmlFor="appointment-type">
                {t("appointments.book.type")}
              </Label>
              <Select
                value={typeUuid}
                onValueChange={(v) => {
                  setTypeUuid(v);
                  setDuration(0);
                  setHostUuid(ANY_HOST);
                  setStartsAt("");
                }}
              >
                <SelectTrigger id="appointment-type">
                  <SelectValue placeholder={t("appointments.book.pick_type")} />
                </SelectTrigger>
                <SelectContent>
                  {active.map((x) => (
                    <SelectItem key={x.uuid} value={x.uuid}>
                      {x.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              {types.isSuccess && active.length === 0 ? (
                <p className="text-muted-foreground text-xs">
                  {t("appointments.book.no_types")}
                </p>
              ) : null}
            </div>
            {type ? (
              <>
                <div className="space-y-1.5">
                  <Label htmlFor="appointment-duration">
                    {t("appointments.book.duration")}
                  </Label>
                  <Select
                    value={String(duration || type.default_duration)}
                    onValueChange={(v) => {
                      setDuration(Number(v));
                      setStartsAt("");
                    }}
                  >
                    <SelectTrigger id="appointment-duration">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {type.duration_options.map((d) => (
                        <SelectItem key={d} value={String(d)}>
                          {t("appointments.minutes", { count: d })}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
                <div className="space-y-1.5">
                  <Label htmlFor="appointment-host">
                    {t("appointments.book.host")}
                  </Label>
                  <Select
                    value={hostUuid}
                    onValueChange={(v) => {
                      setHostUuid(v);
                      setStartsAt("");
                    }}
                  >
                    <SelectTrigger id="appointment-host">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value={ANY_HOST}>
                        {t("appointments.book.any_host")}
                      </SelectItem>
                      {type.hosts.map((h) => (
                        <SelectItem key={h.uuid} value={h.uuid}>
                          {h.name || h.email}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
              </>
            ) : null}
          </div>

          <SlotPicker
            typeUuid={typeUuid}
            duration={duration}
            hostUuid={hostUuid === ANY_HOST ? "" : hostUuid}
            day={day}
            onDayChange={(d) => {
              setDay(d);
              setStartsAt("");
            }}
            value={startsAt}
            onChange={setStartsAt}
          />

          <div className="grid gap-3 sm:grid-cols-2">
            <div className="space-y-1.5 sm:col-span-2">
              <Label htmlFor="appointment-name">
                {t("appointments.book.invitee_name")}
              </Label>
              <Input
                id="appointment-name"
                value={name}
                maxLength={120}
                placeholder={
                  hasLink ? t("appointments.book.from_contact") : undefined
                }
                onChange={(e) => setName(e.target.value)}
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="appointment-email">
                {t("appointments.book.invitee_email")}
              </Label>
              <Input
                id="appointment-email"
                type="email"
                value={email}
                maxLength={254}
                onChange={(e) => setEmail(e.target.value)}
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="appointment-phone">
                {t("appointments.book.invitee_phone")}
              </Label>
              <Input
                id="appointment-phone"
                value={phone}
                maxLength={40}
                onChange={(e) => setPhone(e.target.value)}
              />
            </div>
            <div className="space-y-1.5 sm:col-span-2">
              <Label htmlFor="appointment-notes">
                {t("appointments.book.notes")}
              </Label>
              <Textarea
                id="appointment-notes"
                value={notes}
                maxLength={5000}
                rows={3}
                onChange={(e) => setNotes(e.target.value)}
              />
            </div>
            <label className="flex items-center gap-2 text-sm sm:col-span-2">
              <Switch checked={notify} onCheckedChange={setNotify} />
              {t("appointments.book.notify")}
            </label>
          </div>

          <FormActions>
            <Button
              type="button"
              variant="outline"
              onClick={() => onOpenChange(false)}
            >
              {t("common.cancel")}
            </Button>
            <Button type="submit" disabled={!canSubmit || book.isPending}>
              {t("appointments.book.submit")}
            </Button>
          </FormActions>
        </form>
      </DialogContent>
    </Dialog>
  );
}
