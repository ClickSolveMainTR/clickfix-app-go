"use client";

import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { Check, Copy, KeyRound, Plus, Trash2 } from "lucide-react";
import { useMemo, useState } from "react";

import { StatusChip } from "@/components/common/status-chip";
import { AppDialog } from "@/components/dialogs/app-dialog";
import { DeleteDialog } from "@/components/dialogs/delete-dialog";
import {
  EntityRowActions,
  EntityTable,
  useServerListState,
} from "@/components/entity";
import { FormActions } from "@/components/forms/form-actions";
import { createColumn } from "@/components/tables";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { DatePicker } from "@/components/ui/date-picker";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { isApiError } from "@/lib/api";
import { datetime, relativeDatetime } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

import { SCOPES, scopeKey, WORKFLOW_SCOPES } from "../lib/constants";
import {
  automationIntegrationsService,
  type IntegrationKey,
  type IntegrationKeyInput,
  type IntegrationScope,
  type IssuedIntegrationKey,
} from "../services/automation-integrations.service";

export function useIntegrationsError() {
  const { t } = useLocale();
  return (error: unknown) =>
    appToast.error(
      isApiError(error)
        ? error.message
        : t("automation_integrations.toast.failed"),
    );
}

/** Organization API keys: issue (the secret is shown once) and revoke. */
export function ApiKeysTab({ slug }: { slug: string }) {
  const { t, locale } = useLocale();
  const qc = useQueryClient();
  const onError = useIntegrationsError();
  const key = ["tenant", slug, "api-keys"] as const;
  const listState = useServerListState({ initialPageSize: 50 });
  const { limit, offset } = listState.params;
  const keys = useQuery({
    queryKey: [...key, limit, offset],
    queryFn: () => automationIntegrationsService.keys(slug, limit, offset),
    placeholderData: keepPreviousData,
  });
  const [creating, setCreating] = useState(false);
  const [issued, setIssued] = useState<IssuedIntegrationKey | null>(null);
  const [revoking, setRevoking] = useState<IntegrationKey | null>(null);
  const [now] = useState(() => Date.now());

  const create = useMutation({
    mutationFn: (body: IntegrationKeyInput) =>
      automationIntegrationsService.createKey(slug, body),
    onSuccess: async (out) => {
      setCreating(false);
      setIssued(out);
      await qc.invalidateQueries({ queryKey: key });
    },
    onError,
  });
  const revoke = useMutation({
    mutationFn: (uuid: string) =>
      automationIntegrationsService.revokeKey(slug, uuid),
    onSuccess: async () => {
      setRevoking(null);
      await qc.invalidateQueries({ queryKey: key });
      appToast.success(t("automation_integrations.keys.revoked_toast"));
    },
    onError,
  });

  const columns = useMemo(
    () =>
      [
        createColumn<IntegrationKey>({
          accessorKey: "name",
          labelKey: "automation_integrations.keys.name",
          gridPrimary: true,
          enableHiding: false,
          cell: ({ row }) => (
            <div className="flex flex-wrap items-center gap-2">
              <span className="font-medium">{row.original.name}</span>
              <code className="bg-muted rounded px-1.5 py-0.5 text-xs font-normal">
                {row.original.key_prefix}…
              </code>
            </div>
          ),
        }),
        createColumn<IntegrationKey>({
          id: "status",
          accessorFn: (row) => row.revoked_at ?? "",
          labelKey: "automation_integrations.keys.columns.status",
          cell: ({ row }) => {
            const k = row.original;
            const expired =
              k.expires_at !== null && Date.parse(k.expires_at) <= now;
            return k.revoked_at ? (
              <StatusChip
                label={t("automation_integrations.keys.status.revoked")}
                tone="danger"
              />
            ) : expired ? (
              <StatusChip
                label={t("automation_integrations.keys.status.expired")}
                tone="warning"
              />
            ) : (
              <StatusChip
                label={t("automation_integrations.keys.status.active")}
                tone="success"
              />
            );
          },
        }),
        createColumn<IntegrationKey>({
          id: "scopes",
          accessorFn: (row) => row.scopes.join(" "),
          labelKey: "automation_integrations.keys.scopes",
          cell: ({ row }) => (
            <div className="flex flex-wrap gap-1">
              {row.original.scopes.map((s) => (
                <Badge key={s} variant="outline" className="font-mono">
                  {s}
                </Badge>
              ))}
            </div>
          ),
        }),
        createColumn<IntegrationKey>({
          id: "usage",
          accessorFn: (row) => row.last_used_at ?? "",
          labelKey: "automation_integrations.keys.columns.usage",
          cell: ({ row }) => {
            const k = row.original;
            return (
              <span className="text-muted-foreground text-xs">
                {t("automation_integrations.keys.rate", {
                  count: k.rate_limit,
                })}
                {" · "}
                {k.last_used_at
                  ? t("automation_integrations.keys.last_used", {
                      time: relativeDatetime(k.last_used_at, locale),
                    })
                  : t("automation_integrations.keys.never_used")}
                {k.expires_at &&
                  ` · ${t("automation_integrations.keys.expires", {
                    date: datetime(k.expires_at, "dd.MM.yyyy", locale),
                  })}`}
              </span>
            );
          },
        }),
        createColumn<IntegrationKey>({
          id: "actions",
          labelKey: "common.actions",
          enableHiding: false,
          meta: { headerClassName: "text-right" },
          cell: ({ row }) =>
            row.original.revoked_at ? null : (
              <EntityRowActions
                actions={[
                  {
                    id: "revoke",
                    label: t("automation_integrations.keys.revoke"),
                    icon: Trash2,
                    variant: "destructive",
                    onSelect: () => setRevoking(row.original),
                  },
                ]}
              />
            ),
        }),
      ] as ColumnDef<IntegrationKey, unknown>[],
    [locale, now, t],
  );

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <p className="text-muted-foreground text-sm">
          {t("automation_integrations.keys.hint")}
        </p>
        <Button size="sm" onClick={() => setCreating(true)}>
          <Plus className="size-4" />
          {t("automation_integrations.keys.new")}
        </Button>
      </div>
      <EntityTable
        columns={columns}
        data={keys.data?.items ?? []}
        getRowId={(row) => row.uuid}
        isLoading={keys.isLoading}
        isError={keys.isError}
        errorTitle={t("automation_integrations.error.title")}
        errorDescription={t("automation_integrations.error.description")}
        onRetry={() => void keys.refetch()}
        emptyTitle={t("automation_integrations.keys.empty_title")}
        emptyDescription={t("automation_integrations.keys.empty_description")}
        total={keys.data?.total ?? 0}
        state={listState.tableState}
        features={{
          persistKey: "tenant-api-keys-v1",
          // The endpoint takes `limit` and `offset` only.
          sorting: false,
          globalFilter: false,
          columnFilters: false,
          rowSelection: false,
        }}
      />

      {creating && (
        <KeyDialog
          pending={create.isPending}
          onOpenChange={(o) => !o && setCreating(false)}
          onSubmit={(body) => create.mutate(body)}
        />
      )}
      <IssuedKeyDialog issued={issued} onClose={() => setIssued(null)} />
      <DeleteDialog
        open={Boolean(revoking)}
        title={t("automation_integrations.keys.revoke_title", {
          name: revoking?.name ?? "",
        })}
        description={t("automation_integrations.keys.revoke_description")}
        confirmLabel={t("automation_integrations.keys.revoke")}
        isPending={revoke.isPending}
        onConfirm={() => revoking && revoke.mutate(revoking.uuid)}
        onCancel={() => setRevoking(null)}
      />
    </div>
  );
}

function KeyDialog({
  pending,
  onOpenChange,
  onSubmit,
}: {
  pending: boolean;
  onOpenChange: (open: boolean) => void;
  onSubmit: (body: IntegrationKeyInput) => void;
}) {
  const { t } = useLocale();
  const [name, setName] = useState("");
  const [scopes, setScopes] = useState<IntegrationScope[]>(WORKFLOW_SCOPES);
  const [rate, setRate] = useState("120");
  const [expires, setExpires] = useState("");
  const rateLimit = Number(rate);
  const today = datetime(new Date(), "yyyy-MM-dd");
  const valid =
    name.trim().length > 0 &&
    name.trim().length <= 120 &&
    scopes.length > 0 &&
    Number.isInteger(rateLimit) &&
    rateLimit >= 1 &&
    rateLimit <= 6000 &&
    (!expires || expires > today);

  return (
    <AppDialog
      open
      onOpenChange={onOpenChange}
      title={t("automation_integrations.keys.new")}
      description={t("automation_integrations.keys.form_hint")}
      className="max-h-[90vh] overflow-y-auto sm:max-w-xl"
    >
      <div className="grid gap-4">
        <div className="grid gap-1.5">
          <Label htmlFor="key-name">
            {t("automation_integrations.keys.name")}
          </Label>
          <Input
            id="key-name"
            value={name}
            maxLength={120}
            placeholder="Zapier"
            onChange={(e) => setName(e.target.value)}
          />
        </div>
        <fieldset className="grid gap-2">
          <legend className="mb-1 text-sm font-medium">
            {t("automation_integrations.keys.scopes")}
          </legend>
          <div className="grid gap-2 sm:grid-cols-2">
            {SCOPES.map((s) => (
              <label
                key={s}
                htmlFor={`scope-${s}`}
                className="flex items-start gap-2 text-sm"
              >
                <Checkbox
                  id={`scope-${s}`}
                  className="mt-0.5"
                  checked={scopes.includes(s)}
                  onCheckedChange={(on) =>
                    setScopes((list) =>
                      on ? [...list, s] : list.filter((x) => x !== s),
                    )
                  }
                />
                <span>
                  <code className="text-xs">{s}</code>
                  <span className="text-muted-foreground block text-xs">
                    {t(scopeKey(s))}
                  </span>
                </span>
              </label>
            ))}
          </div>
        </fieldset>
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="grid gap-1.5">
            <Label htmlFor="key-rate">
              {t("automation_integrations.keys.rate_limit")}
            </Label>
            <Input
              id="key-rate"
              type="number"
              min={1}
              max={6000}
              value={rate}
              onChange={(e) => setRate(e.target.value)}
            />
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="key-expires">
              {t("automation_integrations.keys.expires_at")}
            </Label>
            <DatePicker
              id="key-expires"
              value={expires}
              onChange={setExpires}
            />
          </div>
        </div>
        <p className="text-muted-foreground text-xs">
          {t("automation_integrations.keys.membership_hint")}
        </p>
      </div>
      <FormActions>
        <Button variant="outline" onClick={() => onOpenChange(false)}>
          {t("common.cancel")}
        </Button>
        <Button
          disabled={pending || !valid}
          onClick={() =>
            onSubmit({
              name: name.trim(),
              scopes: SCOPES.filter((s) => scopes.includes(s)),
              rate_limit: rateLimit,
              ...(expires
                ? { expires_at: new Date(`${expires}T23:59:59`).toISOString() }
                : {}),
            })
          }
        >
          {t("automation_integrations.keys.create")}
        </Button>
      </FormActions>
    </AppDialog>
  );
}

/** The raw key, once: the API keeps only its hash. */
function IssuedKeyDialog({
  issued,
  onClose,
}: {
  issued: IssuedIntegrationKey | null;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const [copied, setCopied] = useState(false);
  return (
    <AppDialog
      open={Boolean(issued)}
      onOpenChange={(next) => {
        if (!next) {
          setCopied(false);
          onClose();
        }
      }}
      title={t("automation_integrations.secret.title")}
      description={issued?.name}
      className="sm:max-w-xl"
    >
      {issued ? (
        <div className="space-y-4">
          <Alert>
            <KeyRound className="size-4" />
            <AlertTitle>
              {t("automation_integrations.secret.once_title")}
            </AlertTitle>
            <AlertDescription>
              {t("automation_integrations.secret.once_description")}
            </AlertDescription>
          </Alert>
          <div className="grid gap-1.5">
            <Label htmlFor="issued-key">
              {t("automation_integrations.secret.value")}
            </Label>
            <div className="flex gap-2">
              <Input
                id="issued-key"
                readOnly
                value={issued.key}
                className="font-mono text-xs"
                onFocus={(e) => e.currentTarget.select()}
              />
              <Button
                variant="outline"
                size="icon"
                aria-label={t("common.copy")}
                onClick={() => {
                  void navigator.clipboard.writeText(issued.key).then(() => {
                    setCopied(true);
                    window.setTimeout(() => setCopied(false), 2000);
                  });
                }}
              >
                {copied ? (
                  <Check className="size-4" />
                ) : (
                  <Copy className="size-4" />
                )}
              </Button>
            </div>
          </div>
        </div>
      ) : null}
      <FormActions>
        <Button
          onClick={() => {
            setCopied(false);
            onClose();
          }}
        >
          {t("automation_integrations.secret.done")}
        </Button>
      </FormActions>
    </AppDialog>
  );
}
