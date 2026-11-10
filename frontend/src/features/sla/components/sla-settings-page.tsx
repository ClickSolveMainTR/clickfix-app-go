"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { CalendarDays, Clock, Pencil, Plus, Timer, Trash2 } from "lucide-react";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { EntityTable } from "@/components/entity";
import { PageHeader } from "@/components/layout/page-header";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { DatePicker } from "@/components/ui/date-picker";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Permission } from "@/config/permissions";
import { useActiveOrganization } from "@/features/organizations";
import { isApiError } from "@/lib/api";
import { datetime } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

import { formatMinutes, WEEK_DAYS } from "../lib/format";
import {
  slaService,
  type SlaBusinessHours,
  type SlaBusinessHoursInput,
  type SlaHoliday,
  type SlaPolicy,
  type SlaPolicyInput,
} from "../services/sla.service";
import { ContractedCompanies } from "./contracted-companies";
import { HoursDialog } from "./hours-dialog";
import { PolicyDialog } from "./policy-dialog";
import { useDialogs } from "@/providers/dialog-provider";

/**
 * SLA settings (CF-46): policies (conditions → per-priority targets),
 * business hours and holidays. Read with tenant.sla.read, change with
 * tenant.sla.write.
 */
export function SlaSettingsPage() {
  const { t } = useLocale();
  const { active: org } = useActiveOrganization();
  const perms = org?.permissions ?? [];
  const canRead = perms.includes(Permission.TenantSlaRead);
  const canWrite = perms.includes(Permission.TenantSlaWrite);

  return (
    <div className="space-y-6">
      <PageHeader
        icon={<Timer className="size-7" />}
        title={t("sla.title")}
        description={t("sla.description")}
      />
      {!org ? (
        <p className="text-muted-foreground text-sm">
          {t("sla.no_organization")}
        </p>
      ) : !canRead ? (
        <p className="text-muted-foreground text-sm">{t("sla.forbidden")}</p>
      ) : (
        <Tabs defaultValue="policies">
          <TabsList>
            <TabsTrigger value="policies" data-guide="sla-policies">
              {t("sla.tabs.policies")}
            </TabsTrigger>
            <TabsTrigger value="hours" data-guide="sla-business-hours">
              {t("sla.tabs.hours")}
            </TabsTrigger>
            <TabsTrigger value="holidays">{t("sla.tabs.holidays")}</TabsTrigger>
          </TabsList>
          <TabsContent value="policies" className="mt-4">
            <PoliciesTab orgSlug={org.slug} canWrite={canWrite} />
          </TabsContent>
          <TabsContent value="hours" className="mt-4">
            <HoursTab orgSlug={org.slug} canWrite={canWrite} />
          </TabsContent>
          <TabsContent value="holidays" className="mt-4">
            <HolidaysTab orgSlug={org.slug} canWrite={canWrite} />
          </TabsContent>
        </Tabs>
      )}
    </div>
  );
}

function useOnError() {
  const { t } = useLocale();
  return (error: unknown) =>
    appToast.error(isApiError(error) ? error.message : t("sla.toast.failed"));
}

function PoliciesTab({
  orgSlug,
  canWrite,
}: {
  orgSlug: string;
  canWrite: boolean;
}) {
  const { confirmDelete } = useDialogs();
  const { t, locale } = useLocale();
  const qc = useQueryClient();
  const onError = useOnError();
  const key = ["tenant", orgSlug, "sla", "policies"] as const;
  const policies = useQuery({ queryKey: key, queryFn: slaService.policies });
  const hours = useQuery({
    queryKey: ["tenant", orgSlug, "sla", "hours"],
    queryFn: slaService.businessHours,
  });
  const [editing, setEditing] = useState<SlaPolicy | "new" | null>(null);

  const save = useMutation({
    mutationFn: (body: SlaPolicyInput) =>
      editing && editing !== "new"
        ? slaService.updatePolicy(editing.uuid, body)
        : slaService.createPolicy(body),
    onSuccess: async () => {
      setEditing(null);
      await qc.invalidateQueries({ queryKey: key });
      appToast.success(t("sla.toast.saved"));
    },
    onError,
  });
  const remove = useMutation({
    mutationFn: (uuid: string) => slaService.removePolicy(uuid),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: key });
      appToast.success(t("sla.toast.deleted"));
    },
    onError,
  });

  if (policies.isLoading) return <Loading label={t("common.loading")} />;
  if (policies.isError)
    return (
      <ErrorState
        title={t("sla.error.title")}
        description={t("sla.error.description")}
        onRetry={() => void policies.refetch()}
      />
    );
  const items = policies.data ?? [];
  const hoursName = (uuid?: string | null) =>
    hours.data?.find((h) => h.uuid === uuid)?.name ??
    t("sla.fields.business_hours_auto");

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between gap-3">
        <p className="text-muted-foreground text-sm">
          {t("sla.policy.order_hint")}
        </p>
        {canWrite && (
          <Button size="sm" onClick={() => setEditing("new")}>
            <Plus className="size-4" />
            {t("sla.policy.new")}
          </Button>
        )}
      </div>
      {items.length === 0 && (
        <Card>
          <CardContent className="text-muted-foreground py-8 text-center text-sm">
            {t("sla.policy.empty")}
          </CardContent>
        </Card>
      )}
      {items.map((p, i) => (
        <Card key={p.uuid}>
          <CardContent className="space-y-3 py-4">
            <div className="flex items-start gap-3">
              <span className="text-muted-foreground pt-0.5 text-xs tabular-nums">
                {i + 1}.
              </span>
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="font-medium">{p.name}</span>
                  {!p.is_active && (
                    <Badge variant="secondary">
                      {t("sla.policy.inactive")}
                    </Badge>
                  )}
                  <span className="text-muted-foreground flex items-center gap-1 text-xs">
                    <Clock className="size-3" />
                    {hoursName(p.business_hours_uuid)}
                  </span>
                </div>
                {p.description && (
                  <p className="text-muted-foreground text-sm">
                    {p.description}
                  </p>
                )}
                <p className="text-muted-foreground mt-1 text-xs">
                  {conditionSummary(p, t)}
                  {p.contracted_companies > 0 &&
                    ` · ${t("sla.policy.contracted", { count: p.contracted_companies })}`}
                </p>
              </div>
              {canWrite && (
                <div className="flex gap-1">
                  <Button
                    size="icon"
                    variant="ghost"
                    aria-label={t("sla.policy.edit")}
                    onClick={() => setEditing(p)}
                  >
                    <Pencil className="size-4" />
                  </Button>
                  <Button
                    disabled={remove.isPending}
                    size="icon"
                    variant="ghost"
                    aria-label={t("sla.policy.delete")}
                    onClick={async () => {
                      if (
                        await confirmDelete({
                          title: t("sla.policy.confirm_delete"),
                        })
                      )
                        remove.mutate(p.uuid);
                    }}
                  >
                    <Trash2 className="size-4" />
                  </Button>
                </div>
              )}
            </div>
            <div className="grid gap-1 text-xs sm:grid-cols-4">
              {p.targets.map((x) => (
                <div key={x.priority} className="bg-muted/50 rounded-md p-2">
                  <div className="font-medium">
                    {t(`inbox.priority.${x.priority}`)}
                  </div>
                  <div className="text-muted-foreground">
                    {t("sla.metric_short.first_response")}{" "}
                    {formatMinutes(x.first_response_minutes, locale)} ·{" "}
                    {t("sla.metric_short.next_response")}{" "}
                    {formatMinutes(x.next_response_minutes, locale)} ·{" "}
                    {t("sla.metric_short.resolution")}{" "}
                    {formatMinutes(x.resolution_minutes, locale)}
                  </div>
                </div>
              ))}
            </div>
          </CardContent>
        </Card>
      ))}
      <ContractedCompanies
        orgSlug={orgSlug}
        policies={items}
        canWrite={canWrite}
      />
      {editing && (
        <PolicyDialog
          open
          onOpenChange={(o) => !o && setEditing(null)}
          policy={editing === "new" ? null : editing}
          hours={hours.data ?? []}
          pending={save.isPending}
          onSubmit={(body) => save.mutate(body)}
        />
      )}
    </div>
  );
}

function conditionSummary(
  p: SlaPolicy,
  t: (k: string, params?: Record<string, string | number>) => string,
) {
  const c = p.conditions;
  const parts: string[] = [];
  if (c.priorities?.length)
    parts.push(c.priorities.map((x) => t(`inbox.priority.${x}`)).join(", "));
  if (c.channel_types?.length)
    parts.push(c.channel_types.map((x) => t(`sla.channel.${x}`)).join(", "));
  if (c.tag_uuids?.length)
    parts.push(t("sla.policy.tag_count", { count: c.tag_uuids.length }));
  if (c.company_tiers?.length) parts.push(c.company_tiers.join(", "));
  if (c.company_plans?.length) parts.push(c.company_plans.join(", "));
  if (c.min_company_mrr_minor != null)
    parts.push(
      t("sla.policy.min_mrr", { amount: c.min_company_mrr_minor / 100 }),
    );
  if (c.company_uuids?.length)
    parts.push(
      t("sla.policy.company_count", { count: c.company_uuids.length }),
    );
  return parts.length ? parts.join(" · ") : t("sla.policy.all_conversations");
}

function HoursTab({
  orgSlug,
  canWrite,
}: {
  orgSlug: string;
  canWrite: boolean;
}) {
  const { confirmDelete } = useDialogs();
  const { t } = useLocale();
  const qc = useQueryClient();
  const onError = useOnError();
  const key = ["tenant", orgSlug, "sla", "hours"] as const;
  const hours = useQuery({ queryKey: key, queryFn: slaService.businessHours });
  const [editing, setEditing] = useState<SlaBusinessHours | "new" | null>(null);
  const save = useMutation({
    mutationFn: (body: SlaBusinessHoursInput) =>
      editing && editing !== "new"
        ? slaService.updateBusinessHours(editing.uuid, body)
        : slaService.createBusinessHours(body),
    onSuccess: async () => {
      setEditing(null);
      await qc.invalidateQueries({ queryKey: key });
      appToast.success(t("sla.toast.saved"));
    },
    onError,
  });
  const remove = useMutation({
    mutationFn: (uuid: string) => slaService.removeBusinessHours(uuid),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: key });
      appToast.success(t("sla.toast.deleted"));
    },
    onError,
  });
  if (hours.isLoading) return <Loading label={t("common.loading")} />;
  const items = hours.data ?? [];
  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between gap-3">
        <p className="text-muted-foreground text-sm">{t("sla.hours.hint")}</p>
        {canWrite && (
          <Button size="sm" onClick={() => setEditing("new")}>
            <Plus className="size-4" />
            {t("sla.hours.new")}
          </Button>
        )}
      </div>
      {items.length === 0 && (
        <Card>
          <CardContent className="text-muted-foreground py-8 text-center text-sm">
            {t("sla.hours.empty")}
          </CardContent>
        </Card>
      )}
      {items.map((h) => (
        <Card key={h.uuid}>
          <CardContent className="flex items-start gap-3 py-4">
            <div className="min-w-0 flex-1 space-y-1">
              <div className="flex flex-wrap items-center gap-2">
                <span className="font-medium">{h.name}</span>
                {h.is_default && <Badge>{t("sla.hours.default")}</Badge>}
                <span className="text-muted-foreground text-xs">
                  {h.timezone}
                </span>
                {h.use_public_holidays && (
                  <Badge variant="outline">{t("sla.hours.tr_holidays")}</Badge>
                )}
              </div>
              <div className="text-muted-foreground flex flex-wrap gap-x-3 text-xs">
                {WEEK_DAYS.map((d) => {
                  const day = h.working_hours[d];
                  return (
                    <span key={d}>
                      {t(`sla.day_short.${d}`)}{" "}
                      {!day || day.closed
                        ? t("sla.hours.closed")
                        : `${day.open}–${day.close}`}
                    </span>
                  );
                })}
              </div>
            </div>
            {canWrite && (
              <div className="flex gap-1">
                <Button
                  size="icon"
                  variant="ghost"
                  aria-label={t("sla.hours.edit")}
                  onClick={() => setEditing(h)}
                >
                  <Pencil className="size-4" />
                </Button>
                <Button
                  disabled={remove.isPending}
                  size="icon"
                  variant="ghost"
                  aria-label={t("sla.hours.delete")}
                  onClick={async () => {
                    if (
                      await confirmDelete({
                        title: t("sla.hours.confirm_delete"),
                      })
                    )
                      remove.mutate(h.uuid);
                  }}
                >
                  <Trash2 className="size-4" />
                </Button>
              </div>
            )}
          </CardContent>
        </Card>
      ))}
      {editing && (
        <HoursDialog
          open
          onOpenChange={(o) => !o && setEditing(null)}
          hours={editing === "new" ? null : editing}
          pending={save.isPending}
          onSubmit={(body) => save.mutate(body)}
        />
      )}
    </div>
  );
}

function HolidaysTab({
  orgSlug,
  canWrite,
}: {
  orgSlug: string;
  canWrite: boolean;
}) {
  const { confirmDelete } = useDialogs();
  const { t, locale } = useLocale();
  const qc = useQueryClient();
  const onError = useOnError();
  const year = new Date().getFullYear();
  const key = ["tenant", orgSlug, "sla", "holidays"] as const;
  const holidays = useQuery({ queryKey: key, queryFn: slaService.holidays });
  const publicDays = useQuery({
    queryKey: ["sla", "public-holidays", year],
    queryFn: async () => [
      ...(await slaService.publicHolidays(year)),
      ...(await slaService.publicHolidays(year + 1)),
    ],
  });
  const [date, setDate] = useState("");
  const [name, setName] = useState("");
  const [closed, setClosed] = useState(true);
  const [open, setOpen] = useState("09:00");
  const [close, setClose] = useState("13:00");
  const create = useMutation({
    mutationFn: () =>
      slaService.createHoliday({
        date,
        name: name.trim(),
        closed,
        ...(closed ? {} : { open, close }),
      }),
    onSuccess: async () => {
      setDate("");
      setName("");
      await qc.invalidateQueries({ queryKey: key });
      appToast.success(t("sla.toast.saved"));
    },
    onError,
  });
  const remove = useMutation({
    mutationFn: (uuid: string) => slaService.removeHoliday(uuid),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: key });
      appToast.success(t("sla.toast.deleted"));
    },
    onError,
  });
  const today = new Date().toISOString().slice(0, 10);

  const columns = useMemo(
    () =>
      [
        createColumn<SlaHoliday>({
          accessorKey: "date",
          labelKey: "sla.holidays.columns.date",
          enableHiding: false,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {datetime(row.original.date, "dd.MM.yyyy")}
            </span>
          ),
        }),
        createColumn<SlaHoliday>({
          accessorKey: "name",
          labelKey: "sla.holidays.columns.name",
          enableHiding: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="block max-w-[16rem] truncate">
              {row.original.name}
            </span>
          ),
        }),
        createColumn<SlaHoliday>({
          id: "hours",
          accessorFn: (h) => (h.closed ? "" : (h.open ?? "")),
          labelKey: "sla.holidays.columns.hours",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="text-muted-foreground text-xs">
              {row.original.closed
                ? t("sla.holidays.closed")
                : `${row.original.open ?? ""}–${row.original.close ?? ""}`}
            </span>
          ),
        }),
        ...(canWrite
          ? [
              createColumn<SlaHoliday>({
                id: "actions",
                labelKey: "common.actions",
                enableHiding: false,
                enableSorting: false,
                meta: {
                  headerClassName: "text-right",
                  cellClassName: "text-right",
                },
                cell: ({ row }) => (
                  <Button
                    disabled={remove.isPending}
                    size="icon"
                    variant="ghost"
                    aria-label={t("sla.holidays.delete")}
                    onClick={async () => {
                      if (
                        await confirmDelete({
                          title: t("sla.holidays.confirm_delete", {
                            name: row.original.name,
                          }),
                        })
                      )
                        remove.mutate(row.original.uuid);
                    }}
                  >
                    <Trash2 className="size-4" />
                  </Button>
                ),
              }),
            ]
          : []),
      ] as ColumnDef<SlaHoliday, unknown>[],
    [canWrite, confirmDelete, remove, t],
  );

  return (
    <div className="grid gap-6 lg:grid-cols-2">
      <section className="space-y-3">
        <h2 className="flex items-center gap-2 text-sm font-semibold">
          <CalendarDays className="size-4" />
          {t("sla.holidays.own")}
        </h2>
        {canWrite && (
          <Card>
            <CardContent className="grid gap-3 py-4">
              <div className="grid gap-2 sm:grid-cols-2">
                <DatePicker value={date} onChange={setDate} />
                <Input
                  value={name}
                  maxLength={120}
                  placeholder={t("sla.holidays.name_placeholder")}
                  onChange={(e) => setName(e.target.value)}
                />
              </div>
              <div className="flex flex-wrap items-center gap-3">
                <div className="flex items-center gap-2">
                  <Switch
                    id="hol-closed"
                    checked={closed}
                    onCheckedChange={setClosed}
                  />
                  <label htmlFor="hol-closed" className="text-sm">
                    {t("sla.holidays.closed_all_day")}
                  </label>
                </div>
                {!closed && (
                  <div className="flex items-center gap-2">
                    <Input
                      type="time"
                      className="w-28"
                      value={open}
                      aria-label={t("sla.hours.open")}
                      onChange={(e) => setOpen(e.target.value)}
                    />
                    <span className="text-muted-foreground">–</span>
                    <Input
                      type="time"
                      className="w-28"
                      value={close}
                      aria-label={t("sla.hours.close")}
                      onChange={(e) => setClose(e.target.value)}
                    />
                  </div>
                )}
                <Button
                  size="sm"
                  className="ml-auto"
                  disabled={!date || !name.trim() || create.isPending}
                  onClick={() => create.mutate()}
                >
                  <Plus className="size-4" />
                  {t("sla.holidays.add")}
                </Button>
              </div>
            </CardContent>
          </Card>
        )}
        <EntityTable
          columns={columns}
          data={holidays.data ?? []}
          getRowId={(h) => h.uuid}
          isLoading={holidays.isLoading}
          isError={holidays.isError}
          onRetry={() => void holidays.refetch()}
          emptyTitle={t("sla.holidays.empty")}
          emptyDescription=""
          // A few days a year, all in one response: sort and pages in the
          // browser, soonest first.
          manual={{ filtering: false, sorting: false, pagination: false }}
          initialState={{
            pagination: { pageIndex: 0, pageSize: 20 },
            sorting: [{ id: "date", desc: false }],
          }}
          features={{
            toolbar: false,
            globalFilter: false,
            columnFilters: false,
            rowSelection: false,
            viewMode: false,
          }}
        />
      </section>
      <section className="space-y-3">
        <h2 className="text-sm font-semibold">{t("sla.holidays.public")}</h2>
        <p className="text-muted-foreground text-xs">
          {t("sla.holidays.public_hint")}
        </p>
        <ul className="divide-y rounded-md border">
          {(publicDays.data ?? [])
            .filter((d) => d.date >= today)
            .map((d) => (
              <li
                key={d.date}
                className="flex items-center gap-3 p-2.5 text-sm"
              >
                <span className="w-24 tabular-nums">
                  {datetime(d.date, "dd.MM.yyyy")}
                </span>
                <span className="flex-1 truncate">
                  {locale === "tr" ? d.name : d.name_en || d.name}
                </span>
                {d.half_day && (
                  <Badge variant="outline">{t("sla.holidays.half_day")}</Badge>
                )}
              </li>
            ))}
        </ul>
      </section>
    </div>
  );
}
