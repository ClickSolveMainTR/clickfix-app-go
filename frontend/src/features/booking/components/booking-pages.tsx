"use client";

import { useMutation, useQuery } from "@tanstack/react-query";
import { addDays, format, parseISO } from "date-fns";
import {
  CalendarCheck,
  CalendarPlus,
  Clock,
  Download,
  MapPin,
} from "lucide-react";
import Link from "next/link";
import { useMemo, useState, type ReactNode } from "react";

import { Button } from "@/components/ui/button";
import { DatePicker } from "@/components/ui/date-picker";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { Textarea } from "@/components/ui/textarea";
import { routes } from "@/config/routes";
import {
  bookingService,
  type PublicAppointment,
  type PublicBookingSlot,
  type PublicField,
} from "@/features/booking/services/booking.service";
import { LOCATION_KEYS } from "@/features/calendar/lib/layers";
import { COLOR_CLASSES } from "@/features/calendar/components/views/types";
import { ApiError } from "@/lib/api/errors";
import { cn } from "@/lib/utils";
import { dateFnsLocale } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

type T = (key: string, params?: Record<string, string | number>) => string;

function browserZone(): string {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone;
  } catch {
    return "";
  }
}

function errorText(t: T, err: unknown): string {
  if (err instanceof ApiError) {
    if (err.code === "SLOT_UNAVAILABLE") return t("booking.errors.slot_taken");
    if (err.status === 429) return t("booking.errors.rate_limited");
    if (err.code === "APPOINTMENT_POLICY") return t("booking.errors.policy");
    if (err.status === 422) return err.message || t("booking.errors.invalid");
  }
  return t("booking.errors.generic");
}

function Shell({
  title,
  subtitle,
  children,
}: {
  title: string;
  subtitle?: string;
  children: ReactNode;
}) {
  return (
    <main className="bg-muted/30 min-h-dvh px-4 py-10">
      <div className="bg-card mx-auto w-full max-w-2xl rounded-2xl border p-6 shadow-sm sm:p-8">
        <header className="mb-6 space-y-1">
          {subtitle ? (
            <p className="text-muted-foreground text-xs font-medium tracking-wide uppercase">
              {subtitle}
            </p>
          ) : null}
          <h1 className="text-2xl font-semibold">{title}</h1>
        </header>
        {children}
      </div>
    </main>
  );
}

function NotFound() {
  const { t } = useLocale();
  return (
    <Shell title={t("booking.not_found.title")}>
      <p className="text-muted-foreground text-sm">
        {t("booking.not_found.description")}
      </p>
    </Shell>
  );
}

/** `/book/<org>`: the organization's bookable appointment types. */
export function BookingIndexPage({
  org,
  source = "public",
}: {
  org: string;
  source?: "public" | "widget";
}) {
  const { t } = useLocale();
  const index = useQuery({
    queryKey: ["public", "booking", org],
    queryFn: () => bookingService.index(org),
    retry: false,
  });
  if (index.isError) return <NotFound />;
  if (!index.data) {
    return (
      <Shell title="…">
        <Skeleton className="h-32 w-full" />
      </Shell>
    );
  }
  const data = index.data;
  return (
    <Shell title={t("booking.index.title")} subtitle={data.name}>
      {data.types.length === 0 ? (
        <p className="text-muted-foreground text-sm">
          {t("booking.index.empty")}
        </p>
      ) : (
        <ul className="grid gap-3">
          {data.types.map((type) => (
            <li key={type.slug}>
              <Link
                href={`${routes.booking.type(data.slug, type.slug)}${source === "widget" ? "?source=widget" : ""}`}
                className="hover:bg-muted/50 flex items-start gap-3 rounded-xl border p-4 transition-colors"
              >
                <span
                  aria-hidden
                  className={cn(
                    "mt-1.5 size-2.5 shrink-0 rounded-full",
                    COLOR_CLASSES[type.color as keyof typeof COLOR_CLASSES]
                      ?.dot ?? "bg-primary",
                  )}
                />
                <span className="min-w-0 flex-1">
                  <span className="block font-medium">{type.name}</span>
                  {type.description ? (
                    <span className="text-muted-foreground line-clamp-2 block text-sm">
                      {type.description}
                    </span>
                  ) : null}
                  <span className="text-muted-foreground mt-1 flex items-center gap-1 text-xs">
                    <Clock aria-hidden className="size-3" />
                    {type.duration_options
                      .map((d) => t("appointments.minutes", { count: d }))
                      .join(" · ")}
                  </span>
                </span>
              </Link>
            </li>
          ))}
        </ul>
      )}
    </Shell>
  );
}

function Slots({
  slots,
  loading,
  value,
  onChange,
}: {
  slots: PublicBookingSlot[];
  loading: boolean;
  value: string;
  onChange: (v: string) => void;
}) {
  const { t } = useLocale();
  if (loading) {
    return (
      <div className="grid grid-cols-4 gap-1.5">
        {[0, 1, 2, 3].map((i) => (
          <Skeleton key={i} className="h-9" />
        ))}
      </div>
    );
  }
  if (slots.length === 0) {
    return (
      <p className="text-muted-foreground text-sm">{t("booking.no_slots")}</p>
    );
  }
  return (
    <div
      role="listbox"
      aria-label={t("booking.times")}
      className="grid grid-cols-3 gap-1.5 sm:grid-cols-4"
    >
      {slots.map((slot) => (
        <Button
          key={slot.starts_at}
          type="button"
          role="option"
          aria-selected={slot.starts_at === value}
          variant={slot.starts_at === value ? "default" : "outline"}
          className="tabular-nums"
          onClick={() => onChange(slot.starts_at)}
        >
          {format(parseISO(slot.starts_at), "HH:mm")}
        </Button>
      ))}
    </div>
  );
}

function IntakeField({
  field,
  value,
  onChange,
}: {
  field: PublicField;
  value: unknown;
  onChange: (v: unknown) => void;
}) {
  const id = `intake-${field.key}`;
  const label = `${field.label}${field.required ? " *" : ""}`;
  const str = typeof value === "string" ? value : "";
  let control: ReactNode;
  if (field.options.length > 0) {
    control = (
      <Select value={str} onValueChange={onChange}>
        <SelectTrigger id={id}>
          <SelectValue placeholder={field.placeholder || undefined} />
        </SelectTrigger>
        <SelectContent>
          {field.options.map((o) => (
            <SelectItem key={o.value} value={o.value}>
              {o.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    );
  } else if (field.type === "date") {
    control = <DatePicker id={id} value={str} onChange={onChange} />;
  } else if (field.type === "textarea") {
    control = (
      <Textarea
        id={id}
        value={str}
        rows={3}
        placeholder={field.placeholder || undefined}
        onChange={(e) => onChange(e.target.value)}
      />
    );
  } else {
    control = (
      <Input
        id={id}
        value={str}
        type={
          field.type === "email"
            ? "email"
            : field.type === "number"
              ? "number"
              : "text"
        }
        placeholder={field.placeholder || undefined}
        onChange={(e) =>
          onChange(
            (field.type === "number" || field.type === "money") &&
              e.target.value !== ""
              ? Number(e.target.value)
              : e.target.value,
          )
        }
      />
    );
  }
  return (
    <div className="space-y-1.5">
      <Label htmlFor={id}>{label}</Label>
      {control}
      {field.help ? (
        <p className="text-muted-foreground text-xs">{field.help}</p>
      ) : null}
    </div>
  );
}

/** `/book/<org>/<type>`: pick a time, fill the intake form, book. */
export function BookingTypePage({
  org,
  type: typeSlug,
  source = "public",
}: {
  org: string;
  type: string;
  source?: "public" | "widget";
}) {
  const { t, locale } = useLocale();
  const dfLocale = dateFnsLocale(locale);
  const detail = useQuery({
    queryKey: ["public", "booking", org, typeSlug],
    queryFn: () => bookingService.type(org, typeSlug),
    retry: false,
  });
  const [day, setDay] = useState(() => format(new Date(), "yyyy-MM-dd"));
  const [duration, setDuration] = useState(0);
  const [startsAt, setStartsAt] = useState("");
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [notes, setNotes] = useState("");
  const [values, setValues] = useState<Record<string, unknown>>({});
  const to = format(addDays(parseISO(day), 1), "yyyy-MM-dd");
  const slots = useQuery({
    queryKey: ["public", "booking", org, typeSlug, "slots", day, duration],
    queryFn: () => bookingService.slots(org, typeSlug, day, to, duration),
    enabled: detail.isSuccess,
    retry: false,
  });
  const book = useMutation({
    mutationFn: () =>
      bookingService.book(org, typeSlug, {
        starts_at: startsAt,
        duration: duration || undefined,
        timezone: browserZone(),
        name: name.trim(),
        email: email.trim(),
        notes: notes.trim(),
        values,
        source,
      }),
  });
  const zone = useMemo(() => browserZone(), []);

  if (detail.isError) return <NotFound />;
  if (!detail.data) {
    return (
      <Shell title="…">
        <Skeleton className="h-64 w-full" />
      </Shell>
    );
  }
  const { organization, type } = detail.data;
  if (book.data) {
    return <Confirmation appointment={book.data} />;
  }
  // Name / e-mail are always asked; the intake form adds the rest.
  const fields = (type.fields ?? []).filter(
    (f) => f.key !== "name" && f.key !== "email",
  );

  return (
    <Shell title={type.name} subtitle={organization.name}>
      {type.description ? (
        <p className="text-muted-foreground mb-4 text-sm whitespace-pre-wrap">
          {type.description}
        </p>
      ) : null}
      <p className="text-muted-foreground mb-6 flex flex-wrap items-center gap-3 text-sm">
        <span className="flex items-center gap-1">
          <MapPin aria-hidden className="size-4" />
          {t(LOCATION_KEYS[type.location_kind])}
        </span>
        <span className="flex items-center gap-1">
          <Clock aria-hidden className="size-4" />
          {t("appointments.minutes", {
            count: duration || type.default_duration,
          })}
        </span>
      </p>
      <form
        className="space-y-6"
        onSubmit={(e) => {
          e.preventDefault();
          if (startsAt) book.mutate();
        }}
      >
        <section className="space-y-3">
          <div className="grid gap-3 sm:grid-cols-2">
            <div className="space-y-1.5">
              <Label htmlFor="booking-day">{t("booking.day")}</Label>
              <DatePicker
                id="booking-day"
                value={day}
                onChange={(d) => {
                  setDay(d);
                  setStartsAt("");
                }}
              />
            </div>
            {type.duration_options.length > 1 ? (
              <div className="space-y-1.5">
                <Label htmlFor="booking-duration">
                  {t("booking.duration")}
                </Label>
                <Select
                  value={String(duration || type.default_duration)}
                  onValueChange={(v) => {
                    setDuration(Number(v));
                    setStartsAt("");
                  }}
                >
                  <SelectTrigger id="booking-duration">
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
            ) : null}
          </div>
          <p className="text-muted-foreground text-xs">
            {t("booking.times_in_zone", { zone })}
          </p>
          <Slots
            slots={slots.data?.items ?? []}
            loading={slots.isLoading}
            value={startsAt}
            onChange={setStartsAt}
          />
        </section>

        {startsAt ? (
          <section className="space-y-3 border-t pt-6">
            <p className="text-sm font-medium first-letter:uppercase">
              {format(parseISO(startsAt), "PPPP · HH:mm", { locale: dfLocale })}
            </p>
            <div className="grid gap-3 sm:grid-cols-2">
              <div className="space-y-1.5">
                <Label htmlFor="booking-name">{t("booking.name")} *</Label>
                <Input
                  id="booking-name"
                  required
                  maxLength={120}
                  value={name}
                  autoComplete="name"
                  onChange={(e) => setName(e.target.value)}
                />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="booking-email">{t("booking.email")} *</Label>
                <Input
                  id="booking-email"
                  type="email"
                  required
                  maxLength={254}
                  value={email}
                  autoComplete="email"
                  onChange={(e) => setEmail(e.target.value)}
                />
              </div>
            </div>
            {fields.map((f) => (
              <IntakeField
                key={f.key}
                field={f}
                value={values[f.key]}
                onChange={(v) => setValues((prev) => ({ ...prev, [f.key]: v }))}
              />
            ))}
            <div className="space-y-1.5">
              <Label htmlFor="booking-notes">{t("booking.notes")}</Label>
              <Textarea
                id="booking-notes"
                rows={3}
                maxLength={2000}
                value={notes}
                onChange={(e) => setNotes(e.target.value)}
              />
            </div>
            {book.isError ? (
              <p role="alert" className="text-destructive text-sm">
                {errorText(t, book.error)}
              </p>
            ) : null}
            <Button
              type="submit"
              className="w-full sm:w-auto"
              disabled={book.isPending || !name.trim() || !email.trim()}
            >
              <CalendarCheck aria-hidden />
              {t("booking.submit")}
            </Button>
          </section>
        ) : null}
      </form>
    </Shell>
  );
}

function Confirmation({ appointment: a }: { appointment: PublicAppointment }) {
  const { t } = useLocale();
  return (
    <Shell
      title={
        a.requires_confirmation
          ? t("booking.done.pending_title")
          : t("booking.done.title")
      }
      subtitle={a.organization}
    >
      <AppointmentSummary appointment={a} />
      <p className="text-muted-foreground mt-4 text-sm">
        {t("booking.done.email_sent")}
      </p>
      <div className="mt-4 flex flex-wrap gap-2">
        <CalendarLinks appointment={a} />
        <Button asChild variant="ghost">
          <Link href={routes.booking.manage(a.token)}>
            {t("booking.done.manage")}
          </Link>
        </Button>
      </div>
    </Shell>
  );
}

function AppointmentSummary({
  appointment: a,
}: {
  appointment: PublicAppointment;
}) {
  const { t, locale } = useLocale();
  const dfLocale = dateFnsLocale(locale);
  return (
    <dl className="grid gap-3 rounded-xl border p-4 text-sm sm:grid-cols-2">
      <div className="sm:col-span-2">
        <dt className="text-muted-foreground text-xs">{t("booking.when")}</dt>
        <dd className="font-medium first-letter:uppercase">
          {format(parseISO(a.starts_at), "PPPP · HH:mm", { locale: dfLocale })}{" "}
          – {format(parseISO(a.ends_at), "HH:mm")}
        </dd>
      </div>
      <div>
        <dt className="text-muted-foreground text-xs">{t("booking.type")}</dt>
        <dd>{a.type_name}</dd>
      </div>
      <div>
        <dt className="text-muted-foreground text-xs">{t("booking.where")}</dt>
        <dd className="break-words">
          {t(LOCATION_KEYS[a.location_kind])}
          {a.location_value ? ` · ${a.location_value}` : ""}
        </dd>
      </div>
      {a.host_name ? (
        <div>
          <dt className="text-muted-foreground text-xs">{t("booking.host")}</dt>
          <dd>{a.host_name}</dd>
        </div>
      ) : null}
    </dl>
  );
}

function CalendarLinks({ appointment: a }: { appointment: PublicAppointment }) {
  const { t } = useLocale();
  if (a.status === "cancelled") return null;
  return (
    <>
      <Button asChild variant="outline">
        <a href={a.links.ics}>
          <Download aria-hidden />
          {t("booking.add_ics")}
        </a>
      </Button>
      {a.links.google ? (
        <Button asChild variant="outline">
          <a href={a.links.google} target="_blank" rel="noreferrer">
            <CalendarPlus aria-hidden />
            {t("booking.add_google")}
          </a>
        </Button>
      ) : null}
      {a.links.outlook ? (
        <Button asChild variant="outline">
          <a href={a.links.outlook} target="_blank" rel="noreferrer">
            <CalendarPlus aria-hidden />
            {t("booking.add_outlook")}
          </a>
        </Button>
      ) : null}
    </>
  );
}

/** `/book/manage/<token>`: invitee reschedules or cancels. */
export function BookingManagePage({ token }: { token: string }) {
  const { t } = useLocale();
  const [current, setCurrent] = useState(token);
  const manage = useQuery({
    queryKey: ["public", "appointment", current],
    queryFn: () => bookingService.manage(current),
    retry: false,
  });
  const [mode, setMode] = useState<"view" | "reschedule" | "cancel">("view");
  const [reason, setReason] = useState("");
  const [day, setDay] = useState(() => format(new Date(), "yyyy-MM-dd"));
  const [startsAt, setStartsAt] = useState("");
  const to = format(addDays(parseISO(day), 1), "yyyy-MM-dd");
  const slots = useQuery({
    queryKey: ["public", "appointment", current, "slots", day],
    queryFn: () => bookingService.manageSlots(current, day, to),
    enabled: mode === "reschedule",
    retry: false,
  });
  const cancel = useMutation({
    mutationFn: () => bookingService.cancel(current, reason.trim()),
    onSuccess: (a) => {
      setMode("view");
      // Cancelling bumps the token too; the old one is no longer valid.
      setCurrent(a.token);
    },
  });
  const reschedule = useMutation({
    mutationFn: () => bookingService.reschedule(current, startsAt),
    onSuccess: (a) => {
      setMode("view");
      setStartsAt("");
      // The reschedule bumps the token; keep following the new one.
      setCurrent(a.token);
    },
  });

  if (manage.isError) return <NotFound />;
  if (!manage.data) {
    return (
      <Shell title="…">
        <Skeleton className="h-40 w-full" />
      </Shell>
    );
  }
  const a = manage.data;
  const error = cancel.error ?? reschedule.error;
  return (
    <Shell
      title={
        a.status === "cancelled"
          ? t("booking.manage.cancelled_title")
          : t("booking.manage.title")
      }
      subtitle={a.organization}
    >
      <AppointmentSummary appointment={a} />
      {mode === "view" ? (
        <div className="mt-4 flex flex-wrap gap-2">
          <CalendarLinks appointment={a} />
          {a.can_reschedule ? (
            <Button variant="outline" onClick={() => setMode("reschedule")}>
              {t("booking.manage.reschedule")}
            </Button>
          ) : null}
          {a.can_cancel ? (
            <Button
              variant="ghost"
              className="text-destructive"
              onClick={() => setMode("cancel")}
            >
              {t("booking.manage.cancel")}
            </Button>
          ) : null}
        </div>
      ) : mode === "reschedule" ? (
        <section className="mt-6 space-y-3">
          <div className="space-y-1.5">
            <Label htmlFor="manage-day">{t("booking.day")}</Label>
            <DatePicker
              id="manage-day"
              value={day}
              onChange={(d) => {
                setDay(d);
                setStartsAt("");
              }}
            />
          </div>
          <Slots
            slots={slots.data?.items ?? []}
            loading={slots.isLoading}
            value={startsAt}
            onChange={setStartsAt}
          />
          <div className="flex gap-2">
            <Button variant="outline" onClick={() => setMode("view")}>
              {t("common.back")}
            </Button>
            <Button
              disabled={!startsAt || reschedule.isPending}
              onClick={() => reschedule.mutate()}
            >
              {t("booking.manage.reschedule_confirm")}
            </Button>
          </div>
        </section>
      ) : (
        <section className="mt-6 space-y-3">
          <Label htmlFor="manage-reason">{t("booking.manage.reason")}</Label>
          <Textarea
            id="manage-reason"
            rows={3}
            maxLength={1000}
            value={reason}
            onChange={(e) => setReason(e.target.value)}
          />
          <div className="flex gap-2">
            <Button variant="outline" onClick={() => setMode("view")}>
              {t("common.back")}
            </Button>
            <Button
              variant="destructive"
              disabled={cancel.isPending}
              onClick={() => cancel.mutate()}
            >
              {t("booking.manage.cancel_confirm")}
            </Button>
          </div>
        </section>
      )}
      {error ? (
        <p role="alert" className="text-destructive mt-3 text-sm">
          {errorText(t, error)}
        </p>
      ) : null}
    </Shell>
  );
}
