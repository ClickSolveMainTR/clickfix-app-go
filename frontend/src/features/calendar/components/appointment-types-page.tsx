"use client";

import { Copy, Pencil, Plus, Trash2 } from "lucide-react";
import { useState } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import { EntityPage } from "@/components/entity";
import { FormActions } from "@/components/forms/form-actions";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
import { permissions } from "@/config/permissions";
import { AppointmentTypeDialog } from "@/features/calendar/components/appointment-type-dialog";
import { COLOR_CLASSES } from "@/features/calendar/components/views/types";
import {
  useAppointmentSettings,
  useAppointmentTypeMutations,
  useAppointmentTypes,
} from "@/features/calendar/hooks/use-calendar";
import { HOST_MODE_KEYS, LOCATION_KEYS } from "@/features/calendar/lib/layers";
import type {
  AppointmentSettings,
  AppointmentType,
} from "@/features/calendar/types";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

/** Settings → Appointments: module switch, reminders and appointment types. */
export function AppointmentTypesPage() {
  const { t } = useLocale();
  const { hasPermission } = usePermission();
  const canManage = hasPermission(permissions.tenant.appointments.manage);
  const settings = useAppointmentSettings();
  const enabled = Boolean(
    settings.data?.feature_enabled && settings.data.enabled,
  );
  const types = useAppointmentTypes(enabled);
  const { remove } = useAppointmentTypeMutations();
  const [editing, setEditing] = useState<AppointmentType | "new" | null>(null);
  const [deleting, setDeleting] = useState<AppointmentType | null>(null);

  const copy = async (url: string) => {
    try {
      await navigator.clipboard.writeText(url);
      appToast.success(t("appointments.detail.link_copied"));
    } catch {
      appToast.error(t("appointments.detail.copy_failed"));
    }
  };

  return (
    <EntityPage
      title={t("appointments.settings.title")}
      description={t("appointments.settings.subtitle")}
      permission={permissions.tenant.appointments.read}
      actions={
        canManage && enabled ? (
          <Button onClick={() => setEditing("new")}>
            <Plus aria-hidden />
            {t("appointments.types.new")}
          </Button>
        ) : null
      }
    >
      <div className="flex flex-col gap-4">
        {settings.data ? (
          <SettingsCard
            key={JSON.stringify(settings.data)}
            settings={settings.data}
            canManage={canManage}
          />
        ) : settings.isError ? (
          <ErrorState title={t("calendar.error.title")} />
        ) : (
          <Skeleton className="h-40 w-full" />
        )}

        {enabled ? (
          types.isLoading ? (
            <Skeleton className="h-32 w-full" />
          ) : (types.data?.items ?? []).length === 0 ? (
            <EmptyState
              title={t("appointments.types.empty_title")}
              description={t("appointments.types.empty_description")}
              action={
                canManage ? (
                  <Button onClick={() => setEditing("new")}>
                    <Plus aria-hidden />
                    {t("appointments.types.new")}
                  </Button>
                ) : undefined
              }
            />
          ) : (
            <div className="grid gap-3 md:grid-cols-2">
              {(types.data?.items ?? []).map((type) => (
                <Card key={type.uuid} className="gap-2 py-4 shadow-none">
                  <CardHeader className="flex flex-row items-start gap-2 space-y-0 px-4">
                    <span
                      aria-hidden
                      className={cn(
                        "mt-1.5 size-2.5 shrink-0 rounded-full",
                        COLOR_CLASSES[type.color].dot,
                      )}
                    />
                    <div className="min-w-0 flex-1">
                      <CardTitle className="truncate text-base">
                        {type.name}
                      </CardTitle>
                      <p className="text-muted-foreground text-xs">
                        {type.duration_options
                          .map((d) => t("appointments.minutes", { count: d }))
                          .join(" · ")}{" "}
                        · {t(LOCATION_KEYS[type.location_kind])} ·{" "}
                        {t(HOST_MODE_KEYS[type.host_mode])}
                      </p>
                    </div>
                    {!type.is_active ? (
                      <StatusChip label={t("appointments.types.inactive")} />
                    ) : type.is_public ? (
                      <StatusChip
                        label={t("appointments.types.public")}
                        tone="success"
                      />
                    ) : (
                      <StatusChip label={t("appointments.types.internal")} />
                    )}
                  </CardHeader>
                  <CardContent className="space-y-2 px-4">
                    {type.description ? (
                      <p className="text-muted-foreground line-clamp-2 text-sm">
                        {type.description}
                      </p>
                    ) : null}
                    <p className="text-muted-foreground truncate text-xs">
                      {type.host_mode === "team"
                        ? (type.team?.name ?? "—")
                        : type.hosts.map((h) => h.name || h.email).join(", ")}
                    </p>
                    <div className="flex flex-wrap gap-1">
                      {type.is_public && type.is_active ? (
                        <Button
                          size="sm"
                          variant="ghost"
                          onClick={() => void copy(type.booking_url)}
                        >
                          <Copy aria-hidden />
                          {t("appointments.types.copy_link")}
                        </Button>
                      ) : null}
                      {canManage ? (
                        <>
                          <Button
                            size="sm"
                            variant="ghost"
                            onClick={() => setEditing(type)}
                          >
                            <Pencil aria-hidden />
                            {t("appointments.types.edit")}
                          </Button>
                          <Button
                            size="sm"
                            variant="ghost"
                            className="text-destructive"
                            onClick={() => setDeleting(type)}
                          >
                            <Trash2 aria-hidden />
                            {t("common.delete")}
                          </Button>
                        </>
                      ) : null}
                    </div>
                  </CardContent>
                </Card>
              ))}
            </div>
          )
        ) : null}
      </div>

      {editing ? (
        <AppointmentTypeDialog
          type={editing === "new" ? null : editing}
          onOpenChange={(open) => {
            if (!open) setEditing(null);
          }}
        />
      ) : null}
      <ConfirmDialog
        open={deleting !== null}
        title={t("appointments.types.delete_title")}
        description={t("appointments.types.delete_description", {
          name: deleting?.name ?? "",
        })}
        confirmLabel={t("common.delete")}
        variant="destructive"
        isPending={remove.isPending}
        onCancel={() => setDeleting(null)}
        onConfirm={() => {
          if (!deleting) return;
          remove.mutate(deleting.uuid, {
            onSuccess: () => {
              appToast.success(t("appointments.types.deleted"));
              setDeleting(null);
            },
          });
        }}
      />
    </EntityPage>
  );
}

function SettingsCard({
  settings,
  canManage,
}: {
  settings: AppointmentSettings;
  canManage: boolean;
}) {
  const { t } = useLocale();
  const { saveSettings } = useAppointmentTypeMutations();
  const [enabled, setEnabled] = useState(settings.enabled);
  const [publicBooking, setPublicBooking] = useState(settings.public_booking);
  const [timezone, setTimezone] = useState(settings.timezone);
  const [reminders, setReminders] = useState(
    settings.reminder_minutes.join(", "),
  );

  if (!settings.feature_enabled) {
    return (
      <ErrorState
        title={t("calendar.disabled.title")}
        description={t("appointments.settings.plan_required")}
      />
    );
  }

  const save = () => {
    const parsed = reminders
      .split(",")
      .map((v) => Number(v.trim()))
      .filter((v) => Number.isFinite(v) && v > 0);
    saveSettings.mutate(
      {
        enabled,
        public_booking: publicBooking,
        timezone: timezone.trim(),
        reminder_minutes: parsed,
      },
      { onSuccess: () => appToast.success(t("appointments.settings.saved")) },
    );
  };

  return (
    <Card className="gap-3 py-4 shadow-none">
      <CardHeader className="px-4">
        <CardTitle className="text-base">
          {t("appointments.settings.module")}
        </CardTitle>
      </CardHeader>
      <CardContent className="grid gap-4 px-4 sm:grid-cols-2">
        <label className="flex items-center gap-2 text-sm">
          <Switch
            checked={enabled}
            disabled={!canManage}
            onCheckedChange={setEnabled}
          />
          {t("appointments.settings.enabled")}
        </label>
        <label className="flex items-center gap-2 text-sm">
          <Switch
            checked={publicBooking}
            disabled={!canManage}
            onCheckedChange={setPublicBooking}
          />
          {t("appointments.settings.public_booking")}
        </label>
        <div className="space-y-1.5">
          <Label htmlFor="appointments-timezone">
            {t("appointments.settings.timezone")}
          </Label>
          <Input
            id="appointments-timezone"
            value={timezone}
            disabled={!canManage}
            onChange={(e) => setTimezone(e.target.value)}
          />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="appointments-reminders">
            {t("appointments.settings.reminders")}
          </Label>
          <Input
            id="appointments-reminders"
            value={reminders}
            disabled={!canManage}
            onChange={(e) => setReminders(e.target.value)}
          />
          <p className="text-muted-foreground text-xs">
            {t("appointments.settings.reminders_help")}
          </p>
        </div>
        <p className="text-muted-foreground text-xs sm:col-span-2">
          {t("appointments.settings.booking_page")}{" "}
          <a
            href={settings.booking_base_url}
            target="_blank"
            rel="noreferrer"
            className="text-primary break-all hover:underline"
          >
            {settings.booking_base_url}
          </a>
        </p>
        {canManage ? (
          <div className="sm:col-span-2">
            <FormActions sticky={false}>
              <Button onClick={save} disabled={saveSettings.isPending}>
                {t("common.save")}
              </Button>
            </FormActions>
          </div>
        ) : null}
      </CardContent>
    </Card>
  );
}
