"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Import, Plus, Workflow as WorkflowIcon } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { AppDialog } from "@/components/dialogs/app-dialog";
import { FormActions } from "@/components/forms/form-actions";
import { PageHeader } from "@/components/layout/page-header";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
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
  const list = useQuery({
    queryKey: workflowKeys.list(org?.slug),
    queryFn: () => workflowsService.list(100, 0),
    enabled: Boolean(org) && canRead,
  });
  const items = list.data?.items ?? [];

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
      {list.isLoading && <Loading label={t("common.loading")} />}
      {list.isError && (
        <ErrorState
          title={t("workflows.error.title")}
          description={t("workflows.error.description")}
          onRetry={() => void list.refetch()}
        />
      )}
      {list.isSuccess && items.length === 0 && (
        <Card>
          <CardContent className="text-muted-foreground py-10 text-center text-sm">
            {t("workflows.empty")}
          </CardContent>
        </Card>
      )}
      {items.length > 0 && (
        <Card className="overflow-hidden py-0">
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead className="bg-muted/50 text-muted-foreground text-left text-xs">
                <tr>
                  <th className="px-4 py-2.5 font-medium">
                    {t("workflows.columns.name")}
                  </th>
                  <th className="px-4 py-2.5 font-medium">
                    {t("workflows.columns.status")}
                  </th>
                  <th className="px-4 py-2.5 font-medium">
                    {t("workflows.columns.version")}
                  </th>
                  <th className="px-4 py-2.5 font-medium">
                    {t("workflows.columns.last_run")}
                  </th>
                  <th className="px-4 py-2.5 font-medium">
                    {t("workflows.columns.updated")}
                  </th>
                </tr>
              </thead>
              <tbody>
                {items.map((w) => (
                  <WorkflowRow key={w.uuid} workflow={w} />
                ))}
              </tbody>
            </table>
          </div>
        </Card>
      )}
      {creating && <CreateDialog onClose={() => setCreating(false)} />}
      {importing && <ImportDialog onClose={() => setImporting(false)} />}
    </div>
  );
}

function WorkflowRow({ workflow: w }: { workflow: Workflow }) {
  const { t } = useLocale();
  const { active: org } = useActiveOrganization();
  const trigger = w.draft.nodes.find((n) => n.type === "trigger")?.trigger;
  const lastRun = useQuery({
    queryKey: workflowKeys.lastRun(org?.slug, w.uuid),
    queryFn: () => workflowsService.runs(w.uuid, 1, 0),
    enabled: Boolean(org) && w.published_version !== null,
    staleTime: 30_000,
  });
  const run = lastRun.data?.items?.[0];
  return (
    <tr className="hover:bg-muted/40 border-t">
      <td className="px-4 py-3">
        <Link
          href={routes.cms.workflows.detail(w.uuid)}
          className="font-medium hover:underline"
        >
          {w.name}
        </Link>
        {trigger && (
          <p className="text-muted-foreground text-xs">
            {t(`workflows.triggers.${slug(trigger)}`)}
          </p>
        )}
      </td>
      <td className="px-4 py-3">
        <div className="flex flex-wrap gap-1">
          {w.published_version === null ? (
            <Badge variant="secondary">{t("workflows.status.draft")}</Badge>
          ) : w.enabled ? (
            <Badge variant="success">{t("workflows.status.enabled")}</Badge>
          ) : (
            <Badge variant="outline">{t("workflows.status.disabled")}</Badge>
          )}
          {w.published_version !== null &&
            w.published_version !== w.revision && (
              <Badge variant="warning">
                {t("workflows.status.unpublished")}
              </Badge>
            )}
        </div>
      </td>
      <td className="px-4 py-3 text-xs tabular-nums">
        {w.published_version !== null ? `v${w.published_version}` : "—"}
        <span className="text-muted-foreground">
          {" "}
          · {t("workflows.editor.revision", { revision: w.revision })}
        </span>
      </td>
      <td className="px-4 py-3">
        {run ? (
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
        )}
      </td>
      <td className="text-muted-foreground px-4 py-3 text-xs">
        {datetime(w.updated_at, "dd.MM.yyyy HH:mm")}
      </td>
    </tr>
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
