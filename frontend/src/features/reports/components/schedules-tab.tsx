"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { CalendarClock, Plus, Trash2 } from "lucide-react";
import { useState } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { DeleteDialog } from "@/components/dialogs/delete-dialog";
import { AppDialog } from "@/components/dialogs/app-dialog";
import { FormActions } from "@/components/forms/form-actions";
import { Badge } from "@/components/ui/badge";
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
import { useMemberDirectory } from "@/features/tenant-members/hooks/use-member-directory";
import { datetime } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

import { browserTimezone } from "../lib/format";
import {
  reportsService,
  type CustomReport,
  type ReportSchedule,
  type ReportScheduleInput,
} from "../services/reports.service";
import { useReportError } from "../lib/use-report-error";

const FREQUENCIES: ReportSchedule["frequency"][] = [
  "daily",
  "weekly",
  "monthly",
];
const FORMATS: ReportSchedule["format"][] = ["csv", "xlsx"];
const MAX_RECIPIENTS = 20;

/**
 * Scheduled deliveries: a saved report rendered as CSV/XLSX and e-mailed
 * to members as a signed download link (daily at midnight, Mondays or the
 * first of the month in the schedule's zone).
 */
export function SchedulesTab({
  slug,
  canWrite,
}: {
  slug: string;
  canWrite: boolean;
}) {
  const { t, locale } = useLocale();
  const qc = useQueryClient();
  const onError = useReportError();
  const directory = useMemberDirectory();
  const key = ["tenant", slug, "reports", "schedules"] as const;
  const schedules = useQuery({
    queryKey: key,
    queryFn: () => reportsService.schedules(slug),
  });
  const reports = useQuery({
    queryKey: ["tenant", slug, "reports", "custom"],
    queryFn: () => reportsService.list(slug),
  });
  const [creating, setCreating] = useState(false);
  const [deleting, setDeleting] = useState<ReportSchedule | null>(null);

  const create = useMutation({
    mutationFn: (body: ReportScheduleInput) =>
      reportsService.createSchedule(slug, body),
    onSuccess: async () => {
      setCreating(false);
      await qc.invalidateQueries({ queryKey: key });
      appToast.success(t("reports.toast.saved"));
    },
    onError,
  });
  const remove = useMutation({
    mutationFn: (uuid: string) => reportsService.removeSchedule(slug, uuid),
    onSuccess: async () => {
      setDeleting(null);
      await qc.invalidateQueries({ queryKey: key });
      appToast.success(t("reports.toast.deleted"));
    },
    onError,
  });

  if (schedules.isLoading) return <Loading label={t("common.loading")} />;
  if (schedules.isError)
    return (
      <ErrorState
        title={t("reports.error.title")}
        description={t("reports.error.description")}
        onRetry={() => void schedules.refetch()}
      />
    );
  const items = schedules.data ?? [];
  const reportList = reports.data ?? [];

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <p className="text-muted-foreground text-sm">
          {t("reports.schedules.hint")}
        </p>
        {canWrite && (
          <Button
            size="sm"
            disabled={reportList.length === 0}
            title={
              reportList.length === 0
                ? t("reports.schedules.needs_report")
                : undefined
            }
            onClick={() => setCreating(true)}
          >
            <Plus className="size-4" />
            {t("reports.schedules.new")}
          </Button>
        )}
      </div>

      {items.length === 0 ? (
        <EmptyState
          icon={<CalendarClock className="size-6" />}
          title={t("reports.schedules.empty_title")}
          description={t("reports.schedules.empty_description")}
        />
      ) : (
        <div className="overflow-x-auto rounded-md border">
          <table className="w-full min-w-[40rem] text-sm">
            <thead className="bg-muted/50 text-muted-foreground text-left text-xs">
              <tr>
                <th className="px-3 py-2 font-medium">
                  {t("reports.schedules.report")}
                </th>
                <th className="px-3 py-2 font-medium">
                  {t("reports.schedules.frequency")}
                </th>
                <th className="px-3 py-2 font-medium">
                  {t("reports.schedules.format")}
                </th>
                <th className="px-3 py-2 font-medium">
                  {t("reports.schedules.recipients")}
                </th>
                <th className="px-3 py-2 font-medium">
                  {t("reports.schedules.next_run")}
                </th>
                <th className="px-3 py-2" />
              </tr>
            </thead>
            <tbody className="divide-y">
              {items.map((s) => (
                <tr key={s.uuid}>
                  <td className="px-3 py-2">
                    <div className="font-medium">{s.name}</div>
                    {!s.enabled && (
                      <Badge variant="secondary" className="mt-1">
                        {t("reports.schedules.paused")}
                      </Badge>
                    )}
                  </td>
                  <td className="px-3 py-2">
                    {t(`reports.frequency.${s.frequency}`)}
                    <div className="text-muted-foreground text-xs">
                      {s.timezone}
                    </div>
                  </td>
                  <td className="px-3 py-2">
                    <Badge variant="outline">{s.format.toUpperCase()}</Badge>
                  </td>
                  <td
                    className="max-w-[14rem] truncate px-3 py-2"
                    title={s.recipients.map(directory.memberName).join(", ")}
                  >
                    {s.recipients.map(directory.memberName).join(", ")}
                  </td>
                  <td className="px-3 py-2 tabular-nums">
                    {datetime(s.next_run_at, "dd.MM.yyyy HH:mm", locale)}
                  </td>
                  <td className="px-3 py-2 text-right">
                    {canWrite && (
                      <Button
                        size="icon"
                        variant="ghost"
                        aria-label={t("reports.schedules.delete")}
                        onClick={() => setDeleting(s)}
                      >
                        <Trash2 className="size-4" />
                      </Button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {creating && (
        <ScheduleDialog
          reports={reportList}
          members={directory.members}
          pending={create.isPending}
          onOpenChange={(o) => !o && setCreating(false)}
          onSubmit={(body) => create.mutate(body)}
        />
      )}
      <DeleteDialog
        open={Boolean(deleting)}
        title={t("reports.schedules.delete_title")}
        description={t("reports.schedules.delete_description", {
          name: deleting?.name ?? "",
        })}
        isPending={remove.isPending}
        onConfirm={() => deleting && remove.mutate(deleting.uuid)}
        onCancel={() => setDeleting(null)}
      />
    </div>
  );
}

function ScheduleDialog({
  reports,
  members,
  pending,
  onOpenChange,
  onSubmit,
}: {
  reports: CustomReport[];
  members: { value: string; label: string }[];
  pending: boolean;
  onOpenChange: (open: boolean) => void;
  onSubmit: (body: ReportScheduleInput) => void;
}) {
  const { t } = useLocale();
  const [report, setReport] = useState(reports[0]?.uuid ?? "");
  const [frequency, setFrequency] =
    useState<ReportSchedule["frequency"]>("weekly");
  const [format, setFormat] = useState<ReportSchedule["format"]>("xlsx");
  const [timezone, setTimezone] = useState(browserTimezone);
  const [recipients, setRecipients] = useState<string[]>([]);
  const [enabled, setEnabled] = useState(true);
  const valid =
    Boolean(report) &&
    timezone.trim().length > 0 &&
    recipients.length > 0 &&
    recipients.length <= MAX_RECIPIENTS;

  return (
    <AppDialog
      open
      onOpenChange={onOpenChange}
      title={t("reports.schedules.new")}
      description={t("reports.schedules.form_hint")}
      className="max-h-[90vh] overflow-y-auto sm:max-w-xl"
    >
      <div className="grid gap-4">
        <div className="grid gap-1.5">
          <Label htmlFor="schedule-report">
            {t("reports.schedules.report")}
          </Label>
          <Select value={report} onValueChange={setReport}>
            <SelectTrigger id="schedule-report" className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {reports.map((r) => (
                <SelectItem key={r.uuid} value={r.uuid}>
                  {r.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div className="grid gap-4 sm:grid-cols-3">
          <div className="grid gap-1.5">
            <Label htmlFor="schedule-frequency">
              {t("reports.schedules.frequency")}
            </Label>
            <Select
              value={frequency}
              onValueChange={(v) =>
                setFrequency(v as ReportSchedule["frequency"])
              }
            >
              <SelectTrigger id="schedule-frequency" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {FREQUENCIES.map((f) => (
                  <SelectItem key={f} value={f}>
                    {t(`reports.frequency.${f}`)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="schedule-format">
              {t("reports.schedules.format")}
            </Label>
            <Select
              value={format}
              onValueChange={(v) => setFormat(v as ReportSchedule["format"])}
            >
              <SelectTrigger id="schedule-format" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {FORMATS.map((f) => (
                  <SelectItem key={f} value={f}>
                    {f.toUpperCase()}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="schedule-tz">{t("reports.fields.timezone")}</Label>
            <Input
              id="schedule-tz"
              value={timezone}
              onChange={(e) => setTimezone(e.target.value)}
            />
          </div>
        </div>
        <p className="text-muted-foreground text-xs">
          {t(`reports.frequency_hint.${frequency}`)}
        </p>
        <fieldset className="grid gap-2">
          <legend className="mb-1 text-sm font-medium">
            {t("reports.schedules.recipients")}{" "}
            <span className="text-muted-foreground text-xs font-normal">
              ({recipients.length}/{MAX_RECIPIENTS})
            </span>
          </legend>
          <p className="text-muted-foreground text-xs">
            {t("reports.schedules.recipients_hint")}
          </p>
          <div className="grid max-h-56 gap-2 overflow-y-auto rounded-md border p-2 sm:grid-cols-2">
            {members.length === 0 && (
              <p className="text-muted-foreground text-sm">
                {t("reports.schedules.no_members")}
              </p>
            )}
            {members.map((m) => (
              <label
                key={m.value}
                htmlFor={`recipient-${m.value}`}
                className="flex min-w-0 items-center gap-2 text-sm"
              >
                <Checkbox
                  id={`recipient-${m.value}`}
                  checked={recipients.includes(m.value)}
                  disabled={
                    !recipients.includes(m.value) &&
                    recipients.length >= MAX_RECIPIENTS
                  }
                  onCheckedChange={(on) =>
                    setRecipients((list) =>
                      on
                        ? [...list, m.value]
                        : list.filter((x) => x !== m.value),
                    )
                  }
                />
                <span className="truncate">{m.label}</span>
              </label>
            ))}
          </div>
        </fieldset>
        <div className="flex items-center gap-2">
          <Switch
            id="schedule-enabled"
            checked={enabled}
            onCheckedChange={setEnabled}
          />
          <Label htmlFor="schedule-enabled">
            {t("reports.schedules.enabled")}
          </Label>
        </div>
      </div>
      <FormActions>
        <Button variant="outline" onClick={() => onOpenChange(false)}>
          {t("common.cancel")}
        </Button>
        <Button
          disabled={pending || !valid}
          onClick={() =>
            onSubmit({
              report_uuid: report,
              frequency,
              format,
              timezone: timezone.trim(),
              recipients,
              enabled,
            })
          }
        >
          {t("common.save")}
        </Button>
      </FormActions>
    </AppDialog>
  );
}
