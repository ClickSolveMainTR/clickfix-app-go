"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useMemo, useState } from "react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
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
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import {
  notificationPreferencesService,
  type NotificationEventType,
  type NotificationPreferences,
} from "@/services/notification-preferences.service";

const CHANNELS = ["inapp", "email", "push"] as const;
type Channel = (typeof CHANNELS)[number];
type Matrix = Record<string, Record<string, boolean>>;
type Digest = NotificationPreferences["email_digest"];

const QUERY_KEY = ["notification-preferences"] as const;
const EVENTS_KEY = ["notification-preferences", "events"] as const;

const TIMEZONES = [
  "Europe/Istanbul",
  "Europe/London",
  "Europe/Berlin",
  "Asia/Dubai",
  "UTC",
];

/** Stored choice, else the event's default; locked channels are always on. */
export function cellValue(
  matrix: Matrix,
  event: NotificationEventType,
  channel: Channel,
): boolean {
  if (event.locked.includes(channel)) return true;
  const stored = matrix[event.key]?.[channel];
  return stored ?? event.defaults[channel] ?? false;
}

/**
 * Per-event notification matrix (in-app / e-mail / push), quiet hours and
 * the e-mail digest (CF-9). Master switches stay in NotificationPreferencesForm.
 */
export function NotificationMatrixForm() {
  const { t } = useLocale();
  const prefsQuery = useQuery({
    queryKey: QUERY_KEY,
    queryFn: () => notificationPreferencesService.get(),
  });
  const eventsQuery = useQuery({
    queryKey: EVENTS_KEY,
    queryFn: () => notificationPreferencesService.events(),
    staleTime: Infinity,
  });

  if (prefsQuery.isLoading || eventsQuery.isLoading) {
    return (
      <p className="text-muted-foreground text-sm">{t("common.loading")}</p>
    );
  }
  if (!prefsQuery.data || !eventsQuery.data) {
    return (
      <p className="text-destructive text-sm">{t("common.error_generic")}</p>
    );
  }
  return (
    <MatrixEditor
      key={JSON.stringify(prefsQuery.data)}
      prefs={prefsQuery.data}
      events={eventsQuery.data.items}
    />
  );
}

function MatrixEditor({
  prefs,
  events,
}: {
  prefs: NotificationPreferences;
  events: NotificationEventType[];
}) {
  const { t } = useLocale();
  const queryClient = useQueryClient();
  const [matrix, setMatrix] = useState<Matrix>(prefs.event_channels ?? {});
  const [quiet, setQuiet] = useState(prefs.quiet_hours);
  const [digest, setDigest] = useState<Digest>(prefs.email_digest);
  const [digestHour, setDigestHour] = useState(prefs.digest_hour);

  const save = useMutation({
    mutationFn: () =>
      notificationPreferencesService.update({
        event_channels: matrix,
        quiet_hours: quiet,
        email_digest: digest,
        digest_hour: digestHour,
      }),
    onSuccess: (saved) => {
      queryClient.setQueryData(QUERY_KEY, saved);
      toast.success(t("auth.preferences.save_success"));
    },
    onError: (error) => {
      toast.error(
        isApiError(error) && error.message
          ? error.message
          : t("common.error_generic"),
      );
    },
  });

  const groups = useMemo(() => {
    const out = new Map<string, NotificationEventType[]>();
    for (const e of events) {
      out.set(e.group, [...(out.get(e.group) ?? []), e]);
    }
    return [...out.entries()];
  }, [events]);

  const toggle = (
    event: NotificationEventType,
    channel: Channel,
    on: boolean,
  ) => {
    setMatrix((prev) => ({
      ...prev,
      [event.key]: { ...prev[event.key], [channel]: on },
    }));
  };

  return (
    <div className="space-y-8">
      <div className="overflow-x-auto">
        <table className="w-full text-sm">
          <thead>
            <tr className="text-muted-foreground border-b text-left">
              <th className="py-2 pr-4 font-medium">
                {t("auth.preferences.matrix.event")}
              </th>
              {CHANNELS.map((ch) => (
                <th key={ch} className="w-24 py-2 text-center font-medium">
                  {t(`auth.preferences.matrix.channel_${ch}`)}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {groups.map(([group, items]) => [
              <tr key={`g-${group}`}>
                <td
                  colSpan={CHANNELS.length + 1}
                  className="text-muted-foreground pt-4 pb-1 text-xs font-semibold tracking-wide uppercase"
                >
                  {t(`auth.preferences.matrix.group_${group}`)}
                </td>
              </tr>,
              ...items.map((event) => (
                <tr key={event.key} className="border-b last:border-0">
                  <td className="py-2 pr-4">
                    {t(`auth.preferences.matrix.event_${event.key}`)}
                  </td>
                  {CHANNELS.map((ch) => {
                    const locked = event.locked.includes(ch);
                    const id = `np-${event.key}-${ch}`;
                    return (
                      <td key={ch} className="py-2 text-center">
                        <Checkbox
                          id={id}
                          aria-label={`${t(`auth.preferences.matrix.event_${event.key}`)}: ${t(`auth.preferences.matrix.channel_${ch}`)}`}
                          checked={cellValue(matrix, event, ch)}
                          disabled={locked}
                          title={
                            locked
                              ? t("auth.preferences.matrix.locked")
                              : undefined
                          }
                          onCheckedChange={(v) => toggle(event, ch, v === true)}
                        />
                      </td>
                    );
                  })}
                </tr>
              )),
            ])}
          </tbody>
        </table>
      </div>

      <section className="space-y-3">
        <div className="flex items-center justify-between gap-4">
          <div>
            <Label htmlFor="np-quiet">
              {t("auth.preferences.quiet.title")}
            </Label>
            <p className="text-muted-foreground text-xs">
              {t("auth.preferences.quiet.hint")}
            </p>
          </div>
          <Switch
            id="np-quiet"
            checked={quiet.enabled}
            onCheckedChange={(enabled) => setQuiet({ ...quiet, enabled })}
          />
        </div>
        {quiet.enabled ? (
          <div className="grid gap-3 sm:grid-cols-3">
            <div className="space-y-1">
              <Label htmlFor="np-quiet-start">
                {t("auth.preferences.quiet.start")}
              </Label>
              <Input
                id="np-quiet-start"
                type="time"
                value={quiet.start}
                onChange={(e) => setQuiet({ ...quiet, start: e.target.value })}
              />
            </div>
            <div className="space-y-1">
              <Label htmlFor="np-quiet-end">
                {t("auth.preferences.quiet.end")}
              </Label>
              <Input
                id="np-quiet-end"
                type="time"
                value={quiet.end}
                onChange={(e) => setQuiet({ ...quiet, end: e.target.value })}
              />
            </div>
            <div className="space-y-1">
              <Label>{t("auth.preferences.quiet.timezone")}</Label>
              <Select
                value={quiet.timezone}
                onValueChange={(timezone) => setQuiet({ ...quiet, timezone })}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {(TIMEZONES.includes(quiet.timezone)
                    ? TIMEZONES
                    : [quiet.timezone, ...TIMEZONES]
                  ).map((tz) => (
                    <SelectItem key={tz} value={tz}>
                      {tz}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>
        ) : null}
      </section>

      <section className="grid gap-3 sm:grid-cols-2">
        <div className="space-y-1">
          <Label>{t("auth.preferences.digest.title")}</Label>
          <Select value={digest} onValueChange={(v) => setDigest(v as Digest)}>
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {(["off", "hourly", "daily"] as const).map((d) => (
                <SelectItem key={d} value={d}>
                  {t(`auth.preferences.digest.${d}`)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <p className="text-muted-foreground text-xs">
            {t("auth.preferences.digest.hint")}
          </p>
        </div>
        {digest === "daily" ? (
          <div className="space-y-1">
            <Label>{t("auth.preferences.digest.hour")}</Label>
            <Select
              value={String(digestHour)}
              onValueChange={(v) => setDigestHour(Number(v))}
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {Array.from({ length: 24 }, (_, h) => (
                  <SelectItem key={h} value={String(h)}>
                    {`${String(h).padStart(2, "0")}:00`}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        ) : null}
      </section>

      <Button onClick={() => save.mutate()} disabled={save.isPending}>
        {save.isPending
          ? t("auth.preferences.saving")
          : t("auth.preferences.save")}
      </Button>
    </div>
  );
}
