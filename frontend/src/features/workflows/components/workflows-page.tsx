"use client";

import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { Import, Plus, Workflow as WorkflowIcon } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";

import { AppDialog } from "@/components/dialogs/app-dialog";
import { EntityTable, useServerListState } from "@/components/entity";
import { FormActions } from "@/components/forms/form-actions";
import { PageHeader } from "@/components/layout/page-header";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useActiveOrganization } from "@/features/organizations";
import { routingService } from "@/features/routing/services/routing.service";
import { isApiError } from "@/lib/api";
import { datetime } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

import { workflowKeys } from "../hooks/use-workflows";
import { slug, starterGraph } from "../lib/graph";
import { workflowsService, type Workflow } from "../services/workflows.service";
import { FieldSelect } from "./field-select";
import { RunStatus } from "./run-status";

/**
 * Visual workflows of the organization (CF-96): publish state, version and
 * the latest run per workflow. Rows open the canvas editor.
 */
export function WorkflowsPage() {
  const { t } = useLocale();
  const { active: org } = useActiveOrganization();
  const perms = org?.permissions ?? [];
  const canRead = perms.includes(Permission.TenantAutomationsRead);
  const canWrite = perms.includes(Permission.TenantAutomationsWrite);
  const [creating, setCreating] = useState(false);
  const [importing, setImporting] = useState(false);
  const listState = useServerListState({ initialPageSize: 50 });
  const { limit, offset } = listState.params;
  const list = useQuery({
    queryKey: workflowKeys.list(org?.slug, limit, offset),
    queryFn: () => workflowsService.list(limit, offset),
    enabled: Boolean(org) && canRead,
    placeholderData: keepPreviousData,
  });

  const columns = useMemo(
    () =>
      [
        createColumn<Workflow>({
          accessorKey: "name",
          labelKey: "workflows.columns.name",
          gridPrimary: true,
          enableHiding: false,
          cell: ({ row }) => {
            const w = row.original;
            const trigger = w.draft.nodes.find(
              (n) => n.type === "trigger",
            )?.trigger;
            return (
              <div>
                <Link
                  href={routes.cms.workflows.detail(w.uuid)}
                  className="font-medium hover:underline"
                >
                  {w.name}
                </Link>
                {trigger && (
                  <p className="text-muted-foreground text-xs font-normal">
                    {t(`workflows.triggers.${slug(trigger)}`)}
                  </p>
                )}
              </div>
            );
          },
        }),
        createColumn<Workflow>({
          id: "status",
          accessorFn: (row) => row.enabled,
          labelKey: "workflows.columns.status",
          cell: ({ row }) => {
            const w = row.original;
            return (
              <div className="flex flex-wrap gap-1">
                {w.published_version === null ? (
                  <Badge variant="secondary">
                    {t("workflows.status.draft")}
                  </Badge>
                ) : w.enabled ? (
                  <Badge variant="success">
                    {t("workflows.status.enabled")}
                  </Badge>
                ) : (
                  <Badge variant="outline">
                    {t("workflows.status.disabled")}
                  </Badge>
                )}
                {w.published_version !== null &&
                  w.published_version !== w.revision && (
                    <Badge variant="warning">
                      {t("workflows.status.unpublished")}
                    </Badge>
                  )}
              </div>
            );
          },
        }),
        createColumn<Workflow>({
          id: "version",
          accessorFn: (row) => row.published_version ?? 0,
          labelKey: "workflows.columns.version",
          cell: ({ row }) => {
            const w = row.original;
            return (
              <span className="text-xs tabular-nums">
                {w.published_version !== null ? `v${w.published_version}` : "—"}
                <span className="text-muted-foreground">
                  {" "}
                  · {t("workflows.editor.revision", { revision: w.revision })}
                </span>
              </span>
            );
          },
        }),
        createColumn<Workflow>({
          id: "last_run",
          accessorFn: (row) => row.uuid,
          labelKey: "workflows.columns.last_run",
          cell: ({ row }) => <LastRun workflow={row.original} />,
        }),
        createColumn<Workflow>({
          accessorKey: "updated_at",
          labelKey: "workflows.columns.updated",
          cell: ({ row }) => (
            <span className="text-muted-foreground text-xs">
              {datetime(row.original.updated_at, "dd.MM.yyyy HH:mm")}
            </span>
          ),
        }),
      ] as ColumnDef<Workflow, unknown>[],
    [t],
  );

  return (
    <div className="space-y-6">
      <PageHeader
        icon={<WorkflowIcon className="size-7" />}
        title={t("workflows.title")}
        description={t("workflows.description")}
        actions={
          canWrite ? (
            <div className="flex gap-2">
              <Button
                size="sm"
                variant="outline"
                onClick={() => setImporting(true)}
              >
                <Import className="size-4" />
                {t("workflows.import.action")}
              </Button>
              <Button size="sm" onClick={() => setCreating(true)}>
                <Plus className="size-4" />
                {t("workflows.new")}
              </Button>
            </div>
          ) : undefined
        }
      />
      {org && !canRead && (
        <p className="text-muted-foreground text-sm">
          {t("workflows.forbidden")}
        </p>
      )}
      {canRead && (
        <EntityTable
          columns={columns}
          data={list.data?.items ?? []}
          getRowId={(row) => row.uuid}
          isLoading={list.isLoading}
          isError={list.isError}
          errorTitle={t("workflows.error.title")}
          errorDescription={t("workflows.error.description")}
          onRetry={() => void list.refetch()}
          emptyDescription={t("workflows.empty")}
          total={list.data?.total ?? 0}
          state={listState.tableState}
          features={{
            persistKey: "tenant-workflows-v1",
            // The endpoint takes `limit` and `offset` only.
            sorting: false,
            globalFilter: false,
            columnFilters: false,
            rowSelection: false,
          }}
        />
      )}
      {creating && <CreateDialog onClose={() => setCreating(false)} />}
      {importing && <ImportDialog onClose={() => setImporting(false)} />}
    </div>
  );
}

/** Latest run of a published workflow (one small request per row). */
function LastRun({ workflow: w }: { workflow: Workflow }) {
  const { t } = useLocale();
  const { active: org } = useActiveOrganization();
  const lastRun = useQuery({
    queryKey: workflowKeys.lastRun(org?.slug, w.uuid),
    queryFn: () => workflowsService.runs(w.uuid, 1, 0),
    enabled: Boolean(org) && w.published_version !== null,
    staleTime: 30_000,
  });
  const run = lastRun.data?.items?.[0];
  return run ? (
    <span className="flex items-center gap-2">
      <RunStatus status={run.status} />
      <span className="text-muted-foreground text-xs">
        {datetime(run.created_at, "dd.MM HH:mm")}
      </span>
    </span>
  ) : (
    <span className="text-muted-foreground text-xs">
      {lastRun.isLoading ? "…" : t("workflows.runs.never")}
    </span>
  );
}

function CreateDialog({ onClose }: { onClose: () => void }) {
  const { t } = useLocale();
  const router = useRouter();
  const qc = useQueryClient();
  const { active: org } = useActiveOrganization();
  const [name, setName] = useState("");
  const create = useMutation({
    mutationFn: () =>
      workflowsService.create({ name: name.trim(), graph: starterGraph() }),
    onSuccess: async (row) => {
      await qc.invalidateQueries({ queryKey: workflowKeys.all(org?.slug) });
      router.push(routes.cms.workflows.detail(row.uuid));
    },
    onError: (error) =>
      appToast.error(
        isApiError(error) ? error.message : t("workflows.toast.failed"),
      ),
  });
  return (
    <AppDialog
      open
      onOpenChange={(o) => !o && onClose()}
      title={t("workflows.create.title")}
      description={t("workflows.create.description")}
    >
      <form
        className="space-y-4"
        onSubmit={(e) => {
          e.preventDefault();
          if (name.trim()) create.mutate();
        }}
      >
        <div className="space-y-2">
          <Label htmlFor="wf-name">{t("workflows.fields_ui.name")}</Label>
          <Input
            id="wf-name"
            value={name}
            maxLength={200}
            autoFocus
            onChange={(e) => setName(e.target.value)}
          />
        </div>
        <FormActions sticky={false}>
          <Button type="button" variant="outline" onClick={onClose}>
            {t("common.cancel")}
          </Button>
          <Button type="submit" disabled={!name.trim() || create.isPending}>
            {t("workflows.create.submit")}
          </Button>
        </FormActions>
      </form>
    </AppDialog>
  );
}

/** Explicit routing-rule import: creates a published but disabled workflow. */
function ImportDialog({ onClose }: { onClose: () => void }) {
  const { t } = useLocale();
  const router = useRouter();
  const qc = useQueryClient();
  const { active: org } = useActiveOrganization();
  const [rule, setRule] = useState("");
  const rules = useQuery({
    queryKey: ["tenant", org?.slug, "routing", "rules"],
    queryFn: () => routingService.rules(),
    enabled: Boolean(org),
    retry: false,
  });
  const run = useMutation({
    mutationFn: () => workflowsService.importRouting(rule),
    onSuccess: async (row) => {
      await qc.invalidateQueries({ queryKey: workflowKeys.all(org?.slug) });
      appToast.success(t("workflows.import.done"));
      router.push(routes.cms.workflows.detail(row.uuid));
    },
  });
  return (
    <AppDialog
      open
      onOpenChange={(o) => !o && onClose()}
      title={t("workflows.import.title")}
      description={t("workflows.import.description")}
    >
      <div className="space-y-2">
        <Label htmlFor="wf-import-rule">{t("workflows.import.rule")}</Label>
        {rules.isError ? (
          <p className="text-destructive text-sm">
            {t("workflows.import.error")}
          </p>
        ) : (
          <FieldSelect
            id="wf-import-rule"
            value={rule}
            disabled={rules.isLoading}
            placeholder={t("workflows.import.placeholder")}
            onChange={setRule}
            options={(rules.data ?? []).map((r) => ({
              value: r.uuid,
              label: r.is_active
                ? r.name
                : `${r.name} (${t("workflows.status.disabled")})`,
            }))}
          />
        )}
      </div>
      <FormActions sticky={false}>
        <Button type="button" variant="outline" onClick={onClose}>
          {t("common.cancel")}
        </Button>
        <Button
          type="button"
          disabled={!rule || run.isPending}
          onClick={() => run.mutate()}
        >
          {t("workflows.import.submit")}
        </Button>
      </FormActions>
    </AppDialog>
  );
}
