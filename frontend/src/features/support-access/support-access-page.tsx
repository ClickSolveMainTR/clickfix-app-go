"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Eye, LifeBuoy, ShieldCheck, ShieldOff } from "lucide-react";
import { useMemo, useState } from "react";
import { z } from "zod";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import {
  EntityPage,
  EntitySectionCard,
  EntityTable,
  useServerListState,
} from "@/components/entity";
import {
  AppForm,
  AppInput,
  AppRadioGroup,
  AppSelect,
  FormActions,
} from "@/components/forms";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  MODE_LABEL_KEYS,
  STATUS_LABEL_KEYS,
  remaining,
  useGrantSupportAccess,
  useRevokeSupportAccess,
  useSupportGrants,
  type SupportGrant,
} from "@/features/support-access/support-access";
import { useNow } from "@/features/support-access/use-now";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

const DURATIONS = [1, 4, 8, 24, 72, 168];

const STATUS_TONE: Record<
  SupportGrant["status"],
  "success" | "default" | "danger"
> = {
  active: "success",
  expired: "default",
  revoked: "danger",
};

function personName(p: { name: string; surname: string } | null) {
  return p ? `${p.name} ${p.surname}`.trim() : "—";
}

function createGrantSchema(t: (key: string) => string) {
  return z.object({
    mode: z.enum(["read_only", "full"]),
    duration_hours: z.string().min(1, t("support_access.validation.duration")),
    reason: z.string().max(500, t("support_access.validation.reason")),
  });
}
type GrantValues = z.infer<ReturnType<typeof createGrantSchema>>;

function ModeExplainer() {
  const { t } = useLocale();
  return (
    <div className="grid gap-3 md:grid-cols-2">
      <div className="rounded-lg border p-4">
        <p className="flex items-center gap-2 text-sm font-medium">
          <Eye className="size-4" />
          {t("support_access.modes.read_only")}
        </p>
        <p className="text-muted-foreground mt-1 text-sm">
          {t("support_access.modes.read_only_hint")}
        </p>
      </div>
      <div className="rounded-lg border p-4">
        <p className="flex items-center gap-2 text-sm font-medium">
          <ShieldCheck className="size-4" />
          {t("support_access.modes.full")}
        </p>
        <p className="text-muted-foreground mt-1 text-sm">
          {t("support_access.modes.full_hint")}
        </p>
      </div>
    </div>
  );
}

function ActiveGrantCard({
  grant,
  canManage,
}: {
  grant: SupportGrant;
  canManage: boolean;
}) {
  const { t, locale } = useLocale();
  const now = useNow();
  const revoke = useRevokeSupportAccess();
  const [confirm, setConfirm] = useState(false);
  return (
    <EntitySectionCard title={t("support_access.active_title")}>
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center">
        <div className="flex-1 space-y-1">
          <div className="flex flex-wrap items-center gap-2">
            <Badge variant="warning">{t(MODE_LABEL_KEYS[grant.mode])}</Badge>
            <span className="text-sm font-medium tabular-nums">
              {remaining(t, grant.expires_at, now)}
            </span>
          </div>
          <p className="text-muted-foreground text-sm">
            {t("support_access.active_body", {
              date: datetime(grant.expires_at, undefined, locale),
              name: personName(grant.granted_by),
            })}
          </p>
          {grant.reason ? (
            <p className="text-muted-foreground text-sm">“{grant.reason}”</p>
          ) : null}
        </div>
        {canManage ? (
          <Button
            type="button"
            variant="outline"
            className="text-destructive"
            onClick={() => setConfirm(true)}
          >
            <ShieldOff className="size-4" />
            {t("support_access.revoke")}
          </Button>
        ) : null}
      </div>
      <ConfirmDialog
        open={confirm}
        title={t("support_access.revoke_title")}
        description={t("support_access.revoke_description")}
        confirmLabel={t("support_access.revoke")}
        variant="destructive"
        isPending={revoke.isPending}
        onCancel={() => setConfirm(false)}
        onConfirm={() =>
          revoke.mutate(grant.uuid, { onSettled: () => setConfirm(false) })
        }
      />
    </EntitySectionCard>
  );
}

function GrantForm() {
  const { t } = useLocale();
  const grant = useGrantSupportAccess();
  const schema = useMemo(() => createGrantSchema(t), [t]);
  return (
    <EntitySectionCard title={t("support_access.grant_title")}>
      <AppForm<GrantValues>
        className="space-y-4"
        schema={schema}
        defaultValues={{ mode: "read_only", duration_hours: "24", reason: "" }}
        onSubmit={async (values) => {
          await grant.mutateAsync({
            mode: values.mode,
            duration_hours: Number(values.duration_hours),
            reason: values.reason.trim(),
          });
        }}
      >
        <AppRadioGroup
          name="mode"
          label={t("support_access.fields.mode")}
          options={[
            { value: "read_only", label: t("support_access.modes.read_only") },
            { value: "full", label: t("support_access.modes.full") },
          ]}
        />
        <AppSelect
          name="duration_hours"
          label={t("support_access.fields.duration")}
          options={DURATIONS.map((hours) => ({
            value: String(hours),
            label: t("support_access.durations.hours", { count: hours }),
          }))}
        />
        <AppInput
          name="reason"
          label={t("support_access.fields.reason")}
          description={t("support_access.fields.reason_hint")}
          maxLength={500}
        />
        <FormActions>
          <Button type="submit" disabled={grant.isPending}>
            <LifeBuoy className="size-4" />
            {t("support_access.grant")}
          </Button>
        </FormActions>
      </AppForm>
    </EntitySectionCard>
  );
}

/**
 * Support access (CF-14): the organization lets ClickFix support in for a
 * limited time, read-only or full; every support action is in the audit log.
 */
export function SupportAccessPage() {
  const { t, locale } = useLocale();
  const { can } = usePermission();
  const canManage = can(permissions.tenant.supportAccess.manage);
  const listState = useServerListState({
    initialSort: "-created_at",
    initialPageSize: 20,
  });
  const query = useSupportGrants({
    limit: listState.params.limit,
    offset: listState.params.offset,
  });
  const active = query.data?.active ?? null;

  const columns = useMemo(
    () =>
      [
        createColumn<SupportGrant>({
          accessorKey: "created_at",
          labelKey: "support_access.columns.created_at",
          enableSorting: false,
          cell: ({ row }) =>
            datetime(row.original.created_at, undefined, locale),
        }),
        createColumn<SupportGrant>({
          accessorKey: "mode",
          labelKey: "support_access.columns.mode",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => t(MODE_LABEL_KEYS[row.original.mode]),
        }),
        createColumn<SupportGrant>({
          accessorKey: "status",
          labelKey: "support_access.columns.status",
          enableSorting: false,
          cell: ({ row }) => (
            <StatusChip
              label={t(STATUS_LABEL_KEYS[row.original.status])}
              tone={STATUS_TONE[row.original.status]}
            />
          ),
        }),
        createColumn<SupportGrant>({
          accessorKey: "expires_at",
          labelKey: "support_access.columns.expires_at",
          enableSorting: false,
          cell: ({ row }) =>
            datetime(
              row.original.revoked_at ?? row.original.expires_at,
              undefined,
              locale,
            ),
        }),
        createColumn<SupportGrant>({
          id: "granted_by",
          accessorFn: (row) => personName(row.granted_by),
          labelKey: "support_access.columns.granted_by",
          enableSorting: false,
          gridSecondary: true,
          cell: ({ row }) => personName(row.original.granted_by),
        }),
        createColumn<SupportGrant>({
          accessorKey: "reason",
          labelKey: "support_access.columns.reason",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.reason || (
              <span className="text-muted-foreground">—</span>
            ),
        }),
      ] as ColumnDef<SupportGrant, unknown>[],
    [locale, t],
  );

  const total = query.data?.total ?? 0;
  const pageCount = Math.max(
    1,
    Math.ceil(total / (listState.pagination.pageSize || 20)),
  );

  return (
    <EntityPage
      title={t("support_access.title")}
      description={t("support_access.description")}
      permission={permissions.tenant.supportAccess.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("support_access.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.cms.home },
        {
          label: t("layout.section_tenant_settings"),
          href: routes.cms.settings.organization,
        },
        { label: t("support_access.title") },
      ]}
    >
      <div className="space-y-6">
        <ModeExplainer />
        {active ? (
          <ActiveGrantCard grant={active} canManage={canManage} />
        ) : canManage ? (
          <GrantForm />
        ) : null}
        <EntitySectionCard title={t("support_access.history_title")}>
          <EntityTable
            columns={columns}
            data={query.data?.items ?? []}
            getRowId={(row) => row.uuid}
            isLoading={query.isLoading}
            isError={query.isError}
            errorDescription={t("support_access.load_failed")}
            onRetry={() => void query.refetch()}
            emptyTitle={t("support_access.empty_title")}
            emptyDescription={t("support_access.empty_description")}
            pageCount={pageCount}
            state={listState.tableState}
            features={{ persistKey: "tenant-support-access-v1" }}
          />
        </EntitySectionCard>
      </div>
    </EntityPage>
  );
}
