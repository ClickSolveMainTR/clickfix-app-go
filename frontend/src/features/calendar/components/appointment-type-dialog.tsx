"use client";

import { useState, type ReactNode } from "react";

import { FormActions } from "@/components/forms/form-actions";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
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
import {
  CALENDAR_COLORS,
  COLOR_CLASSES,
  COLOR_LABEL_KEYS,
  type CalendarColor,
} from "@/features/calendar/components/views/types";
import { useCalendarFilterOptions } from "@/features/calendar/hooks/use-filter-options";
import { useAppointmentTypeMutations } from "@/features/calendar/hooks/use-calendar";
import { HOST_MODE_KEYS, LOCATION_KEYS } from "@/features/calendar/lib/layers";
import {
  HOST_MODES,
  LOCATION_KINDS,
  type AppointmentHostMode,
  type AppointmentLocationKind,
  type AppointmentType,
  type AppointmentTypeInput,
} from "@/features/calendar/types";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/** Template placeholders the API fills in (usecase.Placeholders). */
const PLACEHOLDERS = [
  "name",
  "type",
  "start",
  "timezone",
  "location",
  "host",
  "organization",
  "manage_url",
]
  .map((p) => `{{${p}}}`)
  .join(", ");

/** System fields an intake form can ask (CF-52 form field format). */
const INTAKE_KEYS = ["phone", "subject", "message"] as const;
type IntakeKey = (typeof INTAKE_KEYS)[number];
const INTAKE_LABEL_KEYS: Record<IntakeKey, string> = {
  phone: "appointments.intake.phone",
  subject: "appointments.intake.subject",
  message: "appointments.intake.message",
};

type IntakeState = Record<IntakeKey, "off" | "optional" | "required">;

function intakeFrom(type: AppointmentType | null): IntakeState {
  const state: IntakeState = { phone: "off", subject: "off", message: "off" };
  for (const f of type?.intake_fields ?? []) {
    if (
      f.source === "system" &&
      (INTAKE_KEYS as readonly string[]).includes(f.key ?? "")
    ) {
      state[f.key as IntakeKey] = f.required ? "required" : "optional";
    }
  }
  return state;
}

const numbers = (raw: string) =>
  raw
    .split(",")
    .map((v) => Number(v.trim()))
    .filter((v) => Number.isInteger(v) && v > 0);

/** Create / edit an appointment type (any meeting an organization defines). */
export function AppointmentTypeDialog({
  type,
  onOpenChange,
}: {
  type: AppointmentType | null;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const { create, update } = useAppointmentTypeMutations();
  const options = useCalendarFilterOptions(true);
  const [name, setName] = useState(type?.name ?? "");
  const [slug, setSlug] = useState(type?.slug ?? "");
  const [description, setDescription] = useState(type?.description ?? "");
  const [color, setColor] = useState<CalendarColor>(type?.color ?? "blue");
  const [durations, setDurations] = useState(
    (type?.duration_options ?? [30]).join(", "),
  );
  const [slotInterval, setSlotInterval] = useState(
    String(type?.slot_interval ?? 30),
  );
  const [bufferBefore, setBufferBefore] = useState(
    String(type?.buffer_before ?? 0),
  );
  const [bufferAfter, setBufferAfter] = useState(
    String(type?.buffer_after ?? 0),
  );
  const [locationKind, setLocationKind] = useState<AppointmentLocationKind>(
    type?.location_kind ?? "video",
  );
  const [locationValue, setLocationValue] = useState(
    type?.location_value ?? "",
  );
  const [hostMode, setHostMode] = useState<AppointmentHostMode>(
    type?.host_mode ?? "round_robin",
  );
  const [teamUuid, setTeamUuid] = useState(type?.team?.uuid ?? "");
  const [hosts, setHosts] = useState<string[]>(
    type?.hosts.map((h) => h.uuid) ?? [],
  );
  const [windowDays, setWindowDays] = useState(
    String(type?.booking_window_days ?? 60),
  );
  const [noticeHours, setNoticeHours] = useState(
    String((type?.min_notice_minutes ?? 240) / 60),
  );
  const [maxPerDay, setMaxPerDay] = useState(String(type?.max_per_day ?? 0));
  const [cancelHours, setCancelHours] = useState(
    String((type?.cancel_notice_minutes ?? 0) / 60),
  );
  const [requiresConfirmation, setRequiresConfirmation] = useState(
    type?.requires_confirmation ?? false,
  );
  const [allowCancel, setAllowCancel] = useState(type?.allow_cancel ?? true);
  const [allowReschedule, setAllowReschedule] = useState(
    type?.allow_reschedule ?? true,
  );
  const [isPublic, setIsPublic] = useState(type?.is_public ?? true);
  const [isActive, setIsActive] = useState(type?.is_active ?? true);
  const [confirmation, setConfirmation] = useState(
    type?.confirmation_template ?? "",
  );
  const [reminder, setReminder] = useState(type?.reminder_template ?? "");
  const [intake, setIntake] = useState<IntakeState>(() => intakeFrom(type));
  // Custom contact fields set through the API are kept as they are.
  const keptFields = (type?.intake_fields ?? []).filter(
    (f) => f.source !== "system",
  );

  const pending = create.isPending || update.isPending;
  const durationList = numbers(durations);

  const submit = () => {
    const body: AppointmentTypeInput = {
      name: name.trim(),
      slug: slug.trim(),
      description: description.trim(),
      color,
      duration_options: durationList,
      default_duration: durationList[0],
      slot_interval: Number(slotInterval) || 30,
      buffer_before: Number(bufferBefore) || 0,
      buffer_after: Number(bufferAfter) || 0,
      location_kind: locationKind,
      location_value: locationValue.trim(),
      host_mode: hostMode,
      team_uuid: hostMode === "team" ? teamUuid || null : null,
      host_uuids: hostMode === "team" ? [] : hosts,
      booking_window_days: Number(windowDays) || 60,
      min_notice_minutes: Math.round((Number(noticeHours) || 0) * 60),
      max_per_day: Number(maxPerDay) || 0,
      cancel_notice_minutes: Math.round((Number(cancelHours) || 0) * 60),
      requires_confirmation: requiresConfirmation,
      allow_cancel: allowCancel,
      allow_reschedule: allowReschedule,
      is_public: isPublic,
      is_active: isActive,
      confirmation_template: confirmation,
      reminder_template: reminder,
      intake_fields: [
        ...INTAKE_KEYS.filter((k) => intake[k] !== "off").map((k) => ({
          key: k,
          source: "system" as const,
          required: intake[k] === "required",
        })),
        ...keptFields,
      ],
    };
    const done = () => {
      appToast.success(t("appointments.types.saved"));
      onOpenChange(false);
    };
    if (type) update.mutate({ uuid: type.uuid, body }, { onSuccess: done });
    else create.mutate(body, { onSuccess: done });
  };

  return (
    <Dialog open onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90dvh] overflow-y-auto sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>
            {type ? t("appointments.types.edit") : t("appointments.types.new")}
          </DialogTitle>
          <DialogDescription>
            {t("appointments.types.form_description")}
          </DialogDescription>
        </DialogHeader>
        <form
          className="space-y-5"
          onSubmit={(e) => {
            e.preventDefault();
            submit();
          }}
        >
          <Section title={t("appointments.types.section_basics")}>
            <Field id="type-name" label={t("appointments.types.name")}>
              <Input
                id="type-name"
                value={name}
                maxLength={120}
                required
                placeholder={t("appointments.types.name_placeholder")}
                onChange={(e) => setName(e.target.value)}
              />
            </Field>
            <Field id="type-slug" label={t("appointments.types.slug")}>
              <Input
                id="type-slug"
                value={slug}
                maxLength={64}
                placeholder={t("appointments.types.slug_placeholder")}
                onChange={(e) => setSlug(e.target.value.toLowerCase())}
              />
            </Field>
            <Field
              id="type-description"
              label={t("appointments.types.description")}
              wide
            >
              <Textarea
                id="type-description"
                value={description}
                maxLength={2000}
                rows={2}
                onChange={(e) => setDescription(e.target.value)}
              />
            </Field>
            <div className="space-y-1.5 sm:col-span-2">
              <span className="text-sm font-medium">
                {t("appointments.types.color")}
              </span>
              <div className="flex flex-wrap gap-1.5">
                {CALENDAR_COLORS.map((c) => (
                  <button
                    key={c}
                    type="button"
                    aria-label={t(COLOR_LABEL_KEYS[c])}
                    aria-pressed={color === c}
                    onClick={() => setColor(c)}
                    className={cn(
                      "size-6 rounded-full ring-offset-2 outline-none focus-visible:ring-2",
                      COLOR_CLASSES[c].dot,
                      color === c && "ring-foreground ring-2",
                    )}
                  />
                ))}
              </div>
            </div>
          </Section>

          <Section title={t("appointments.types.section_time")}>
            <Field
              id="type-durations"
              label={t("appointments.types.durations")}
              help={t("appointments.types.durations_help")}
            >
              <Input
                id="type-durations"
                value={durations}
                onChange={(e) => setDurations(e.target.value)}
              />
            </Field>
            <Field id="type-interval" label={t("appointments.types.interval")}>
              <Input
                id="type-interval"
                type="number"
                min={5}
                max={240}
                value={slotInterval}
                onChange={(e) => setSlotInterval(e.target.value)}
              />
            </Field>
            <Field
              id="type-buffer-before"
              label={t("appointments.types.buffer_before")}
            >
              <Input
                id="type-buffer-before"
                type="number"
                min={0}
                max={240}
                value={bufferBefore}
                onChange={(e) => setBufferBefore(e.target.value)}
              />
            </Field>
            <Field
              id="type-buffer-after"
              label={t("appointments.types.buffer_after")}
            >
              <Input
                id="type-buffer-after"
                type="number"
                min={0}
                max={240}
                value={bufferAfter}
                onChange={(e) => setBufferAfter(e.target.value)}
              />
            </Field>
            <Field id="type-window" label={t("appointments.types.window")}>
              <Input
                id="type-window"
                type="number"
                min={1}
                max={365}
                value={windowDays}
                onChange={(e) => setWindowDays(e.target.value)}
              />
            </Field>
            <Field id="type-notice" label={t("appointments.types.notice")}>
              <Input
                id="type-notice"
                type="number"
                min={0}
                step={0.5}
                value={noticeHours}
                onChange={(e) => setNoticeHours(e.target.value)}
              />
            </Field>
            <Field
              id="type-max"
              label={t("appointments.types.max_per_day")}
              help={t("appointments.types.max_per_day_help")}
            >
              <Input
                id="type-max"
                type="number"
                min={0}
                max={100}
                value={maxPerDay}
                onChange={(e) => setMaxPerDay(e.target.value)}
              />
            </Field>
          </Section>

          <Section title={t("appointments.types.section_where")}>
            <Field id="type-location" label={t("appointments.types.location")}>
              <Select
                value={locationKind}
                onValueChange={(v) =>
                  setLocationKind(v as AppointmentLocationKind)
                }
              >
                <SelectTrigger id="type-location">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {LOCATION_KINDS.map((k) => (
                    <SelectItem key={k} value={k}>
                      {t(LOCATION_KEYS[k])}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>
            <Field
              id="type-location-value"
              label={t("appointments.types.location_value")}
              help={
                locationKind === "video"
                  ? t("appointments.types.video_help")
                  : undefined
              }
            >
              <Input
                id="type-location-value"
                value={locationValue}
                maxLength={500}
                placeholder={
                  locationKind === "custom_url" ? "https://" : undefined
                }
                onChange={(e) => setLocationValue(e.target.value)}
              />
            </Field>
            <Field
              id="type-host-mode"
              label={t("appointments.types.host_mode")}
            >
              <Select
                value={hostMode}
                onValueChange={(v) => setHostMode(v as AppointmentHostMode)}
              >
                <SelectTrigger id="type-host-mode">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {HOST_MODES.map((m) => (
                    <SelectItem key={m} value={m}>
                      {t(HOST_MODE_KEYS[m])}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>
            {hostMode === "team" ? (
              <Field id="type-team" label={t("appointments.types.team")}>
                <Select value={teamUuid} onValueChange={setTeamUuid}>
                  <SelectTrigger id="type-team">
                    <SelectValue
                      placeholder={t("appointments.types.pick_team")}
                    />
                  </SelectTrigger>
                  <SelectContent>
                    {options.teams.map((team) => (
                      <SelectItem key={team.uuid} value={team.uuid}>
                        {team.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </Field>
            ) : (
              <fieldset className="space-y-1 sm:col-span-2">
                <legend className="text-sm font-medium">
                  {t("appointments.types.hosts")}
                </legend>
                <div className="grid max-h-40 gap-1 overflow-y-auto rounded-md border p-2 sm:grid-cols-2">
                  {options.members.map((m) => (
                    <label
                      key={m.uuid}
                      className="hover:bg-muted flex items-center gap-2 rounded px-1 py-1 text-sm"
                    >
                      <Checkbox
                        checked={hosts.includes(m.uuid)}
                        onCheckedChange={(v) =>
                          setHosts((prev) =>
                            v === true
                              ? [...prev, m.uuid]
                              : prev.filter((x) => x !== m.uuid),
                          )
                        }
                      />
                      <span className="truncate">{m.name}</span>
                    </label>
                  ))}
                </div>
              </fieldset>
            )}
          </Section>

          <Section title={t("appointments.types.section_intake")}>
            {INTAKE_KEYS.map((k) => (
              <Field
                key={k}
                id={`type-intake-${k}`}
                label={t(INTAKE_LABEL_KEYS[k])}
              >
                <Select
                  value={intake[k]}
                  onValueChange={(v) =>
                    setIntake((prev) => ({
                      ...prev,
                      [k]: v as IntakeState[IntakeKey],
                    }))
                  }
                >
                  <SelectTrigger id={`type-intake-${k}`}>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="off">
                      {t("appointments.intake.off")}
                    </SelectItem>
                    <SelectItem value="optional">
                      {t("appointments.intake.optional")}
                    </SelectItem>
                    <SelectItem value="required">
                      {t("appointments.intake.required")}
                    </SelectItem>
                  </SelectContent>
                </Select>
              </Field>
            ))}
          </Section>

          <Section title={t("appointments.types.section_policy")}>
            <Toggle
              checked={requiresConfirmation}
              onChange={setRequiresConfirmation}
              label={t("appointments.types.requires_confirmation")}
            />
            <Toggle
              checked={isPublic}
              onChange={setIsPublic}
              label={t("appointments.types.is_public")}
            />
            <Toggle
              checked={allowCancel}
              onChange={setAllowCancel}
              label={t("appointments.types.allow_cancel")}
            />
            <Toggle
              checked={allowReschedule}
              onChange={setAllowReschedule}
              label={t("appointments.types.allow_reschedule")}
            />
            <Field
              id="type-cancel-notice"
              label={t("appointments.types.cancel_notice")}
            >
              <Input
                id="type-cancel-notice"
                type="number"
                min={0}
                step={0.5}
                value={cancelHours}
                onChange={(e) => setCancelHours(e.target.value)}
              />
            </Field>
            <Toggle
              checked={isActive}
              onChange={setIsActive}
              label={t("appointments.types.is_active")}
            />
          </Section>

          <Section title={t("appointments.types.section_messages")}>
            <Field
              id="type-confirmation"
              label={t("appointments.types.confirmation_template")}
              help={`${t("appointments.types.template_help")} ${PLACEHOLDERS}`}
              wide
            >
              <Textarea
                id="type-confirmation"
                value={confirmation}
                maxLength={4000}
                rows={3}
                onChange={(e) => setConfirmation(e.target.value)}
              />
            </Field>
            <Field
              id="type-reminder"
              label={t("appointments.types.reminder_template")}
              wide
            >
              <Textarea
                id="type-reminder"
                value={reminder}
                maxLength={4000}
                rows={3}
                onChange={(e) => setReminder(e.target.value)}
              />
            </Field>
          </Section>

          <FormActions>
            <Button
              type="button"
              variant="outline"
              onClick={() => onOpenChange(false)}
            >
              {t("common.cancel")}
            </Button>
            <Button
              type="submit"
              disabled={pending || !name.trim() || durationList.length === 0}
            >
              {t("common.save")}
            </Button>
          </FormActions>
        </form>
      </DialogContent>
    </Dialog>
  );
}

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <fieldset className="space-y-3">
      <legend className="text-muted-foreground mb-2 text-xs font-semibold tracking-wide uppercase">
        {title}
      </legend>
      <div className="grid gap-3 sm:grid-cols-2">{children}</div>
    </fieldset>
  );
}

function Field({
  id,
  label,
  help,
  wide,
  children,
}: {
  id: string;
  label: string;
  help?: string;
  wide?: boolean;
  children: ReactNode;
}) {
  return (
    <div className={cn("space-y-1.5", wide && "sm:col-span-2")}>
      <Label htmlFor={id}>{label}</Label>
      {children}
      {help ? <p className="text-muted-foreground text-xs">{help}</p> : null}
    </div>
  );
}

function Toggle({
  checked,
  onChange,
  label,
}: {
  checked: boolean;
  onChange: (v: boolean) => void;
  label: string;
}) {
  return (
    <label className="flex items-center gap-2 text-sm">
      <Switch checked={checked} onCheckedChange={onChange} />
      {label}
    </label>
  );
}
