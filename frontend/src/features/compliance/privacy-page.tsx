"use client";

import type { ColumnDef } from "@tanstack/react-table";
import {
  Archive,
  ClipboardList,
  Download,
  FileJson,
  FileSpreadsheet,
  FileText,
  ShieldCheck,
  Timer,
  TriangleAlert,
  UserX,
} from "lucide-react";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { useMemo, useState } from "react";
import { z } from "zod";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import { AppDialog } from "@/components/dialogs/app-dialog";
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
  AppSwitch,
  FormActions,
} from "@/components/forms";
import { createColumn } from "@/components/tables";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { AsyncCombobox } from "@/components/ui/async-combobox";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  KIND_LABEL_KEYS,
  STATUS_LABEL_KEYS,
  downloadExport,
  searchContacts,
  useContactConsents,
  useCreateDataSubjectRequest,
  useDataSubjectRequests,
  useProcessingInventory,
  useRecordConsent,
  useRetentionPolicy,
  useSaveRetentionPolicy,
  type ContactConsent,
  type DataSubjectKind,
  type DataSubjectRequestItem,
  type ProcessingInventory,
  type RetentionPolicy,
} from "@/features/compliance/compliance";
import { useContact } from "@/features/contacts/hooks/use-contacts";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

type Translate = (
  key: string,
  vars?: Record<string, string | number>,
) => string;

const TABS = [
  { id: "requests", labelKey: "privacy.tabs.requests", icon: ClipboardList },
  { id: "retention", labelKey: "privacy.tabs.retention", icon: Timer },
  { id: "consents", labelKey: "privacy.tabs.consents", icon: ShieldCheck },
  { id: "inventory", labelKey: "privacy.tabs.inventory", icon: FileText },
] as const;

const STATUS_TONE: Record<
  DataSubjectRequestItem["status"],
  "default" | "warning" | "success"
> = {
  pending: "default",
  processing: "warning",
  completed: "success",
};

type Picked = { uuid: string; label: string };

/** Contact search; seeds the label of a contact preselected via `?contact=`. */
function ContactPicker({
  value,
  onChange,
}: {
  value: Picked | null;
  onChange: (v: Picked | null) => void;
}) {
  const { t } = useLocale();
  const [labels, setLabels] = useState<Record<string, string>>({});
  const initialOptions = useMemo(
    () => (value ? [{ value: value.uuid, label: value.label }] : []),
    [value],
  );
  return (
    <AsyncCombobox
      value={value?.uuid ?? ""}
      initialOptions={initialOptions}
      loadOptions={async (q) => {
        const options = await searchContacts(q);
        setLabels((prev) => ({
          ...prev,
          ...Object.fromEntries(options.map((o) => [o.value, o.label])),
        }));
        return options;
      }}
      onValueChange={(uuid) =>
        onChange(uuid ? { uuid, label: labels[uuid] ?? uuid } : null)
      }
      placeholder={t("privacy.contact_placeholder")}
      searchPlaceholder={t("privacy.contact_search")}
      emptyText={t("privacy.contact_empty")}
      className="w-full sm:max-w-md"
      clearable
    />
  );
}

/** Contact from `?contact=` (contact detail link), resolved to a label. */
function useLinkedContact(): [Picked | null, (v: Picked | null) => void] {
  const params = useSearchParams();
  const linked = params.get("contact");
  const contact = useContact(linked);
  const [picked, setPicked] = useState<Picked | null | undefined>(undefined);
  if (picked !== undefined) return [picked, setPicked];
  if (linked && contact.data) {
    return [
      {
        uuid: linked,
        label: contact.data.name || contact.data.email || linked,
      },
      setPicked,
    ];
  }
  return [null, setPicked];
}

function ErasureDialog({
  contact,
  open,
  onOpenChange,
}: {
  contact: Picked;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const create = useCreateDataSubjectRequest();
  const [typed, setTyped] = useState("");
  const [ack, setAck] = useState(false);
  const ready = typed.trim() === contact.label.trim() && ack;
  const close = (next: boolean) => {
    if (create.isPending) return;
    if (!next) {
      setTyped("");
      setAck(false);
    }
    onOpenChange(next);
  };
  return (
    <AppDialog
      open={open}
      onOpenChange={close}
      title={t("privacy.erase.title")}
      description={t("privacy.erase.description")}
      footer={
        <>
          <Button
            variant="outline"
            onClick={() => close(false)}
            disabled={create.isPending}
          >
            {t("common.cancel")}
          </Button>
          <Button
            variant="destructive"
            disabled={!ready || create.isPending}
            onClick={() =>
              create.mutate(
                { contact: contact.uuid, kind: "erase" },
                { onSuccess: () => close(false) },
              )
            }
          >
            <UserX className="size-4" />
            {t("privacy.erase.confirm")}
          </Button>
        </>
      }
    >
      <div className="space-y-4">
        <Alert variant="destructive">
          <TriangleAlert className="size-4" />
          <AlertTitle>{t("privacy.erase.irreversible")}</AlertTitle>
          <AlertDescription>{t("privacy.erase.scope")}</AlertDescription>
        </Alert>
        <div className="space-y-2">
          <Label htmlFor="erase-confirm">
            {t("privacy.erase.type_label", { value: contact.label })}
          </Label>
          <Input
            id="erase-confirm"
            value={typed}
            onChange={(e) => setTyped(e.target.value)}
            autoComplete="off"
            spellCheck={false}
          />
        </div>
        <label className="flex items-start gap-2 text-sm">
          <Checkbox
            checked={ack}
            onCheckedChange={(v) => setAck(v === true)}
            className="mt-0.5"
          />
          <span>{t("privacy.erase.ack")}</span>
        </label>
      </div>
    </AppDialog>
  );
}

function NewRequestCard() {
  const { t } = useLocale();
  const [contact, setContact] = useLinkedContact();
  const [kind, setKind] = useState<DataSubjectKind>("export");
  const [erasing, setErasing] = useState(false);
  const create = useCreateDataSubjectRequest();
  return (
    <EntitySectionCard
      title={t("privacy.new_request.title")}
      description={t("privacy.new_request.description")}
    >
      <div className="space-y-4">
        <div className="space-y-2">
          <Label>{t("privacy.fields.contact")}</Label>
          <ContactPicker value={contact} onChange={setContact} />
        </div>
        <div className="grid gap-3 md:grid-cols-2">
          {(["export", "erase"] as const).map((k) => (
            <button
              key={k}
              type="button"
              onClick={() => setKind(k)}
              aria-pressed={kind === k}
              className="hover:bg-muted/50 aria-pressed:border-primary aria-pressed:ring-primary/30 rounded-lg border p-4 text-left transition aria-pressed:ring-2"
            >
              <p className="flex items-center gap-2 text-sm font-medium">
                {k === "export" ? (
                  <Archive className="size-4" />
                ) : (
                  <UserX className="text-destructive size-4" />
                )}
                {t(KIND_LABEL_KEYS[k])}
              </p>
              <p className="text-muted-foreground mt-1 text-sm">
                {t(
                  k === "export"
                    ? "privacy.kind.export_hint"
                    : "privacy.kind.erase_hint",
                )}
              </p>
            </button>
          ))}
        </div>
        <FormActions sticky={false} className="border-0 bg-transparent p-0">
          <Button
            type="button"
            variant={kind === "erase" ? "destructive" : "default"}
            disabled={!contact || create.isPending}
            onClick={() => {
              if (!contact) return;
              if (kind === "erase") setErasing(true);
              else create.mutate({ contact: contact.uuid, kind });
            }}
          >
            {kind === "erase" ? (
              <UserX className="size-4" />
            ) : (
              <Archive className="size-4" />
            )}
            {t(
              kind === "erase"
                ? "privacy.new_request.erase"
                : "privacy.new_request.export",
            )}
          </Button>
        </FormActions>
      </div>
      {contact ? (
        <ErasureDialog
          contact={contact}
          open={erasing}
          onOpenChange={setErasing}
        />
      ) : null}
    </EntitySectionCard>
  );
}

function personName(p: { name: string; surname: string } | null) {
  return p ? `${p.name} ${p.surname}`.trim() || "—" : "—";
}

function RequestsTab() {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const listState = useServerListState({
    initialSort: "-created_at",
    initialPageSize: 20,
  });
  const query = useDataSubjectRequests({
    limit: listState.params.limit,
    offset: listState.params.offset,
  });
  const [downloading, setDownloading] = useState<string | null>(null);

  const columns = useMemo(
    () =>
      [
        createColumn<DataSubjectRequestItem>({
          accessorKey: "created_at",
          labelKey: "privacy.columns.created_at",
          enableSorting: false,
          cell: ({ row }) =>
            datetime(row.original.created_at, undefined, locale),
        }),
        createColumn<DataSubjectRequestItem>({
          id: "contact",
          accessorFn: (row) => row.contact.name || row.contact.email,
          labelKey: "privacy.columns.contact",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => {
            const c = row.original.contact;
            return (
              <div className="min-w-0">
                <p className="truncate font-medium">
                  {c.name || c.email || "—"}
                </p>
                {c.erased ? (
                  <Badge variant="secondary">{t("privacy.erased")}</Badge>
                ) : c.name && c.email ? (
                  <p className="text-muted-foreground truncate text-xs">
                    {c.email}
                  </p>
                ) : null}
              </div>
            );
          },
        }),
        createColumn<DataSubjectRequestItem>({
          accessorKey: "kind",
          labelKey: "privacy.columns.kind",
          enableSorting: false,
          cell: ({ row }) => t(KIND_LABEL_KEYS[row.original.kind]),
        }),
        createColumn<DataSubjectRequestItem>({
          accessorKey: "status",
          labelKey: "privacy.columns.status",
          enableSorting: false,
          cell: ({ row }) => (
            <StatusChip
              label={t(STATUS_LABEL_KEYS[row.original.status])}
              tone={STATUS_TONE[row.original.status]}
            />
          ),
        }),
        createColumn<DataSubjectRequestItem>({
          id: "requested_by",
          accessorFn: (row) => personName(row.requested_by),
          labelKey: "privacy.columns.requested_by",
          enableSorting: false,
          gridSecondary: true,
          cell: ({ row }) => personName(row.original.requested_by),
        }),
        createColumn<DataSubjectRequestItem>({
          accessorKey: "completed_at",
          labelKey: "privacy.columns.completed_at",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.completed_at ? (
              datetime(row.original.completed_at, undefined, locale)
            ) : (
              <span className="text-muted-foreground">—</span>
            ),
        }),
        createColumn<DataSubjectRequestItem>({
          id: "download",
          labelKey: "privacy.columns.download",
          enableSorting: false,
          cell: ({ row }) => {
            const r = row.original;
            if (r.kind !== "export" || r.status !== "completed") return null;
            if (!r.download_url) {
              return (
                <span className="text-muted-foreground text-xs">
                  {t("privacy.download_expired")}
                </span>
              );
            }
            return (
              <div className="flex flex-col items-start gap-0.5">
                <Button
                  size="sm"
                  variant="outline"
                  disabled={downloading === r.uuid}
                  onClick={async () => {
                    setDownloading(r.uuid);
                    try {
                      await downloadExport(slug, r.download_url as string);
                    } catch {
                      appToast.error(t("privacy.download_failed"));
                    } finally {
                      setDownloading(null);
                    }
                  }}
                >
                  <Download className="size-4" />
                  {t("privacy.download")}
                </Button>
                {r.expires_at ? (
                  <span className="text-muted-foreground text-xs">
                    {t("privacy.download_until", {
                      date: datetime(r.expires_at, undefined, locale),
                    })}
                  </span>
                ) : null}
              </div>
            );
          },
        }),
      ] as ColumnDef<DataSubjectRequestItem, unknown>[],
    [downloading, locale, slug, t],
  );

  const total = query.data?.total ?? 0;

  return (
    <div className="space-y-6">
      <NewRequestCard />
      <EntitySectionCard
        title={t("privacy.requests_title")}
        description={t("privacy.requests_description")}
      >
        <EntityTable
          columns={columns}
          data={query.data?.items ?? []}
          getRowId={(row) => row.uuid}
          isLoading={query.isLoading}
          isError={query.isError}
          errorDescription={t("privacy.load_failed")}
          onRetry={() => void query.refetch()}
          emptyTitle={t("privacy.requests_empty_title")}
          emptyDescription={t("privacy.requests_empty_description")}
          total={total}
          state={listState.tableState}
          features={{
            persistKey: "tenant-privacy-requests-v1",
            globalFilter: false,
            columnFilters: false,
          }}
        />
      </EntitySectionCard>
    </div>
  );
}

const RETENTION_FIELDS = [
  {
    key: "conversation_days",
    labelKey: "privacy.retention.conversations",
    hintKey: "privacy.retention.conversations_hint",
  },
  {
    key: "attachment_days",
    labelKey: "privacy.retention.attachments",
    hintKey: "privacy.retention.attachments_hint",
  },
  {
    key: "ai_log_days",
    labelKey: "privacy.retention.ai_logs",
    hintKey: "privacy.retention.ai_logs_hint",
  },
] as const;

type RetentionKey = (typeof RETENTION_FIELDS)[number]["key"];

function createRetentionSchema(t: Translate) {
  const days = z
    .string()
    .regex(/^\d{1,5}$/, t("privacy.validation.days"))
    .refine((v) => Number(v) >= 1 && Number(v) <= 36500, {
      message: t("privacy.validation.days"),
    });
  const category = z.object({ enabled: z.boolean(), days: z.string() });
  return z
    .object({
      action: z.enum(["anonymize", "delete"]),
      conversation_days: category,
      attachment_days: category,
      ai_log_days: category,
    })
    .superRefine((v, ctx) => {
      for (const { key } of RETENTION_FIELDS) {
        if (!v[key].enabled) continue;
        const r = days.safeParse(v[key].days);
        if (!r.success) {
          ctx.addIssue({
            code: "custom",
            path: [key, "days"],
            message: r.error.issues[0]?.message ?? "",
          });
        }
      }
    });
}
type RetentionValues = z.infer<ReturnType<typeof createRetentionSchema>>;

function toValues(p: RetentionPolicy): RetentionValues {
  const category = (n: number) => ({
    enabled: n > 0,
    days: String(n > 0 ? n : 365),
  });
  return {
    action: p.action,
    conversation_days: category(p.conversation_days),
    attachment_days: category(p.attachment_days),
    ai_log_days: category(p.ai_log_days),
  };
}

function RetentionTab() {
  const { t } = useLocale();
  const query = useRetentionPolicy();
  const save = useSaveRetentionPolicy();
  const schema = useMemo(() => createRetentionSchema(t), [t]);
  if (query.isLoading) return <Skeleton className="h-80" />;
  if (query.isError || !query.data) {
    return (
      <ErrorState
        title={t("privacy.load_failed")}
        onRetry={() => void query.refetch()}
      />
    );
  }
  return (
    <EntitySectionCard
      title={t("privacy.retention.title")}
      description={t("privacy.retention.description")}
    >
      <AppForm<RetentionValues>
        key={JSON.stringify(query.data)}
        className="space-y-6"
        schema={schema}
        defaultValues={toValues(query.data)}
        onSubmit={async (v) => {
          const days = (k: RetentionKey) =>
            v[k].enabled ? Number(v[k].days) : 0;
          await save.mutateAsync({
            action: v.action,
            conversation_days: days("conversation_days"),
            attachment_days: days("attachment_days"),
            ai_log_days: days("ai_log_days"),
          });
        }}
      >
        {(form) => (
          <>
            <div className="divide-y rounded-lg border">
              {RETENTION_FIELDS.map(({ key, labelKey, hintKey }) => {
                const enabled = form.watch(`${key}.enabled`);
                return (
                  <div
                    key={key}
                    className="grid gap-3 p-4 sm:grid-cols-[1fr_12rem] sm:items-start"
                  >
                    <AppSwitch
                      name={`${key}.enabled`}
                      label={t(labelKey)}
                      description={t(hintKey)}
                    />
                    {enabled ? (
                      <AppInput
                        name={`${key}.days`}
                        label={t("privacy.retention.days")}
                        inputMode="numeric"
                      />
                    ) : (
                      <p className="text-muted-foreground text-sm sm:pt-1">
                        {t("privacy.retention.off")}
                      </p>
                    )}
                  </div>
                );
              })}
            </div>
            <AppRadioGroup
              name="action"
              label={t("privacy.retention.action")}
              options={[
                {
                  value: "anonymize",
                  label: t("privacy.retention.anonymize"),
                },
                { value: "delete", label: t("privacy.retention.delete") },
              ]}
            />
            {form.watch("action") === "delete" ? (
              <Alert variant="destructive">
                <TriangleAlert className="size-4" />
                <AlertDescription>
                  {t("privacy.retention.delete_warning")}
                </AlertDescription>
              </Alert>
            ) : (
              <p className="text-muted-foreground text-sm">
                {t("privacy.retention.anonymize_hint")}
              </p>
            )}
            <p className="text-muted-foreground text-sm">
              {t("privacy.retention.schedule_hint")}
            </p>
            <FormActions sticky={false} className="border-0 bg-transparent p-0">
              <Button type="submit" disabled={save.isPending}>
                {t("common.save")}
              </Button>
            </FormActions>
          </>
        )}
      </AppForm>
    </EntitySectionCard>
  );
}

function createConsentSchema(t: Translate) {
  const text = z
    .string()
    .trim()
    .min(1, t("privacy.validation.required"))
    .max(100, t("privacy.validation.max_100"));
  return z.object({
    purpose: text,
    granted: z.enum(["true", "false"]),
    notice_version: text,
    source: text,
  });
}
type ConsentValues = z.infer<ReturnType<typeof createConsentSchema>>;

function RecordConsentDialog({
  contact,
  open,
  onOpenChange,
}: {
  contact: Picked;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const record = useRecordConsent(contact.uuid);
  const schema = useMemo(() => createConsentSchema(t), [t]);
  return (
    <AppDialog
      open={open}
      onOpenChange={(next) => !record.isPending && onOpenChange(next)}
      title={t("privacy.consents.record_title")}
      description={t("privacy.consents.record_description", {
        name: contact.label,
      })}
    >
      <AppForm<ConsentValues>
        className="space-y-4"
        schema={schema}
        defaultValues={{
          purpose: "",
          granted: "true",
          notice_version: "",
          source: "panel",
        }}
        onSubmit={async (v) => {
          await record.mutateAsync({
            purpose: v.purpose,
            granted: v.granted === "true",
            notice_version: v.notice_version,
            source: v.source,
          });
          onOpenChange(false);
        }}
      >
        <AppInput
          name="purpose"
          label={t("privacy.consents.purpose")}
          description={t("privacy.consents.purpose_hint")}
          maxLength={100}
        />
        <AppRadioGroup
          name="granted"
          label={t("privacy.consents.decision")}
          options={[
            { value: "true", label: t("privacy.consents.granted") },
            { value: "false", label: t("privacy.consents.withdrawn") },
          ]}
        />
        <AppInput
          name="notice_version"
          label={t("privacy.consents.notice_version")}
          description={t("privacy.consents.notice_version_hint")}
          maxLength={100}
        />
        <AppInput
          name="source"
          label={t("privacy.consents.source")}
          description={t("privacy.consents.source_hint")}
          maxLength={100}
        />
        <FormActions sticky={false} className="border-0 bg-transparent p-0">
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
            disabled={record.isPending}
          >
            {t("common.cancel")}
          </Button>
          <Button type="submit" disabled={record.isPending}>
            {t("privacy.consents.record")}
          </Button>
        </FormActions>
      </AppForm>
    </AppDialog>
  );
}

function ConsentsTab() {
  const { t, locale } = useLocale();
  const [contact, setContact] = useLinkedContact();
  const [recording, setRecording] = useState(false);
  const listState = useServerListState({
    initialSort: "-created_at",
    initialPageSize: 20,
  });
  const query = useContactConsents(contact?.uuid ?? null, {
    limit: listState.params.limit,
    offset: listState.params.offset,
  });
  const columns = useMemo(
    () =>
      [
        createColumn<ContactConsent>({
          accessorKey: "created_at",
          labelKey: "privacy.consents.columns.created_at",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.created_at
              ? datetime(row.original.created_at, undefined, locale)
              : "—",
        }),
        createColumn<ContactConsent>({
          accessorKey: "purpose",
          labelKey: "privacy.consents.columns.purpose",
          enableSorting: false,
          gridPrimary: true,
        }),
        createColumn<ContactConsent>({
          accessorKey: "granted",
          labelKey: "privacy.consents.columns.decision",
          enableSorting: false,
          cell: ({ row }) => (
            <StatusChip
              label={t(
                row.original.granted
                  ? "privacy.consents.granted"
                  : "privacy.consents.withdrawn",
              )}
              tone={row.original.granted ? "success" : "danger"}
            />
          ),
        }),
        createColumn<ContactConsent>({
          accessorKey: "notice_version",
          labelKey: "privacy.consents.columns.notice_version",
          enableSorting: false,
          gridSecondary: true,
        }),
        createColumn<ContactConsent>({
          accessorKey: "source",
          labelKey: "privacy.consents.columns.source",
          enableSorting: false,
        }),
      ] as ColumnDef<ContactConsent, unknown>[],
    [locale, t],
  );
  const total = query.data?.total ?? 0;
  return (
    <EntitySectionCard
      title={t("privacy.consents.title")}
      description={t("privacy.consents.description")}
    >
      <div className="space-y-4">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-end">
          <div className="flex-1 space-y-2">
            <Label>{t("privacy.fields.contact")}</Label>
            <ContactPicker value={contact} onChange={setContact} />
          </div>
          {contact ? (
            <Button type="button" onClick={() => setRecording(true)}>
              <ShieldCheck className="size-4" />
              {t("privacy.consents.record")}
            </Button>
          ) : null}
        </div>
        {contact ? (
          <EntityTable
            columns={columns}
            data={query.data?.items ?? []}
            getRowId={(row) => row.uuid ?? ""}
            isLoading={query.isLoading}
            isError={query.isError}
            errorDescription={t("privacy.load_failed")}
            onRetry={() => void query.refetch()}
            emptyTitle={t("privacy.consents.empty_title")}
            emptyDescription={t("privacy.consents.empty_description")}
            total={total}
            state={listState.tableState}
            features={{
              persistKey: "tenant-privacy-consents-v1",
              globalFilter: false,
              columnFilters: false,
            }}
          />
        ) : (
          <p className="text-muted-foreground rounded-lg border border-dashed p-6 text-center text-sm">
            {t("privacy.consents.pick_contact")}
          </p>
        )}
      </div>
      {contact ? (
        <RecordConsentDialog
          contact={contact}
          open={recording}
          onOpenChange={setRecording}
        />
      ) : null}
    </EntitySectionCard>
  );
}

function saveFile(name: string, type: string, body: string) {
  const href = URL.createObjectURL(new Blob([body], { type }));
  const a = document.createElement("a");
  a.href = href;
  a.download = name;
  a.click();
  URL.revokeObjectURL(href);
}

function csvCell(v: string) {
  return /[",\n;]/.test(v) ? `"${v.replace(/"/g, '""')}"` : v;
}

function inventoryCsv(t: Translate, inv: ProcessingInventory) {
  const header = [
    t("privacy.inventory.category"),
    t("privacy.inventory.purpose"),
    t("privacy.inventory.legal_basis"),
    t("privacy.inventory.subjects"),
    t("privacy.inventory.data"),
  ];
  const rows = inv.items.map((i) => [
    i.category,
    i.purpose,
    i.legal_basis,
    (i.subjects ?? []).join(", "),
    (i.data ?? []).join(", "),
  ]);
  // BOM keeps Turkish characters intact when the file opens in Excel.
  return (
    "﻿" + [header, ...rows].map((r) => r.map(csvCell).join(",")).join("\r\n")
  );
}

function retentionSummary(t: Translate, days: number) {
  return days > 0
    ? t("privacy.inventory.retention_days", { days })
    : t("privacy.retention.off");
}

function InventoryTab() {
  const { t } = useLocale();
  const query = useProcessingInventory();
  if (query.isLoading) return <Skeleton className="h-80" />;
  if (query.isError || !query.data) {
    return (
      <ErrorState
        title={t("privacy.load_failed")}
        onRetry={() => void query.refetch()}
      />
    );
  }
  const inv = query.data;
  return (
    <EntitySectionCard
      title={t("privacy.inventory.title")}
      description={t("privacy.inventory.description")}
      actions={
        <div className="flex flex-wrap gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={() =>
              saveFile(
                "verbis-envanter.csv",
                "text/csv;charset=utf-8",
                inventoryCsv(t, inv),
              )
            }
          >
            <FileSpreadsheet className="size-4" />
            CSV
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={() =>
              saveFile(
                "verbis-envanter.json",
                "application/json",
                JSON.stringify(inv, null, 2),
              )
            }
          >
            <FileJson className="size-4" />
            JSON
          </Button>
        </div>
      }
    >
      <div className="space-y-4">
        {inv.review_required ? (
          <Alert>
            <TriangleAlert className="size-4" />
            <AlertTitle>{t("privacy.inventory.review_required")}</AlertTitle>
            <AlertDescription>{inv.note}</AlertDescription>
          </Alert>
        ) : null}
        <div className="grid gap-3 lg:grid-cols-3">
          {inv.items.map((item) => (
            <div
              key={item.category}
              className="space-y-3 rounded-lg border p-4"
            >
              <p className="font-medium">{item.category}</p>
              <dl className="space-y-2 text-sm">
                <div>
                  <dt className="text-muted-foreground text-xs">
                    {t("privacy.inventory.purpose")}
                  </dt>
                  <dd>{item.purpose}</dd>
                </div>
                <div>
                  <dt className="text-muted-foreground text-xs">
                    {t("privacy.inventory.legal_basis")}
                  </dt>
                  <dd>{item.legal_basis}</dd>
                </div>
                {item.subjects?.length ? (
                  <div>
                    <dt className="text-muted-foreground text-xs">
                      {t("privacy.inventory.subjects")}
                    </dt>
                    <dd className="mt-1 flex flex-wrap gap-1">
                      {item.subjects.map((s) => (
                        <Badge key={s} variant="secondary">
                          {s}
                        </Badge>
                      ))}
                    </dd>
                  </div>
                ) : null}
                {item.data?.length ? (
                  <div>
                    <dt className="text-muted-foreground text-xs">
                      {t("privacy.inventory.data")}
                    </dt>
                    <dd className="mt-1 flex flex-wrap gap-1">
                      {item.data.map((s) => (
                        <Badge key={s} variant="outline">
                          {s}
                        </Badge>
                      ))}
                    </dd>
                  </div>
                ) : null}
              </dl>
            </div>
          ))}
        </div>
        <div className="rounded-lg border p-4 text-sm">
          <p className="mb-2 font-medium">
            {t("privacy.inventory.retention_title")}
          </p>
          <ul className="text-muted-foreground space-y-1">
            <li>
              {t("privacy.retention.conversations")}:{" "}
              {retentionSummary(t, inv.retention.conversation_days)}
            </li>
            <li>
              {t("privacy.retention.attachments")}:{" "}
              {retentionSummary(t, inv.retention.attachment_days)}
            </li>
            <li>
              {t("privacy.retention.ai_logs")}:{" "}
              {retentionSummary(t, inv.retention.ai_log_days)}
            </li>
          </ul>
        </div>
      </div>
    </EntitySectionCard>
  );
}

/**
 * KVKK / GDPR tools of the active organization (CF-127): data subject
 * export and erasure, retention, consent records and the VERBİS draft.
 * Every endpoint requires `tenant.settings.write`.
 */
export function PrivacyPage() {
  const { t } = useLocale();
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const requested = searchParams.get("tab") ?? "";
  const tab = TABS.some((x) => x.id === requested) ? requested : TABS[0].id;
  const selectTab = (value: string) => {
    const params = new URLSearchParams(searchParams.toString());
    params.set("tab", value);
    router.replace(`${pathname}?${params.toString()}`, { scroll: false });
  };
  return (
    <EntityPage
      title={t("privacy.title")}
      description={t("privacy.description")}
      permission={permissions.tenant.settings.write}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("privacy.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.cms.home },
        {
          label: t("layout.section_tenant_settings"),
          href: routes.cms.settings.organization,
        },
        { label: t("privacy.title") },
      ]}
    >
      <Tabs value={tab} onValueChange={selectTab} className="gap-4">
        <div className="-mx-1 overflow-x-auto px-1 pb-1">
          <TabsList className="w-max">
            {TABS.map(({ id, labelKey, icon: Icon }) => (
              <TabsTrigger key={id} value={id} className="flex-none gap-1.5">
                <Icon className="size-4" />
                {t(labelKey)}
              </TabsTrigger>
            ))}
          </TabsList>
        </div>
        <TabsContent value="requests">
          {tab === "requests" && <RequestsTab />}
        </TabsContent>
        <TabsContent value="retention">
          {tab === "retention" && <RetentionTab />}
        </TabsContent>
        <TabsContent value="consents">
          {tab === "consents" && <ConsentsTab />}
        </TabsContent>
        <TabsContent value="inventory">
          {tab === "inventory" && <InventoryTab />}
        </TabsContent>
      </Tabs>
    </EntityPage>
  );
}
