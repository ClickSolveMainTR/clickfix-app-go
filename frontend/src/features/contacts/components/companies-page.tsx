"use client";

import { useMemo, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import type { ColumnDef } from "@tanstack/react-table";
import { Building2, Plus, Trash2 } from "lucide-react";

import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import { EntityTable, useServerListState } from "@/components/entity";
import { PageHeader } from "@/components/layout/page-header";
import { createColumn } from "@/components/tables";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { formatMinor } from "@/features/billing/lib/money";
import { useLocale } from "@/providers/locale-provider";

import { CompanyHealthBadge } from "./company-health-badge";
import { CompanyHealthCard } from "./company-health-card";
import { CompanyRevenueCard } from "./company-revenue-card";
import { ContactsIOToolbar } from "./contacts-io-toolbar";
import { OwnerPicker } from "./owner-picker";

import {
  useCompanyList,
  useCompanyMutations,
  useContactsScope,
} from "../hooks/use-contacts";
import type { Company } from "../services/contacts.service";

/** Companies: B2B accounts the contacts belong to (CF-84). */
export function CompaniesPage() {
  const { t, locale } = useLocale();
  const router = useRouter();
  const scope = useContactsScope();
  const listState = useServerListState({
    initialPageSize: 50,
    searchDebounceMs: 250,
  });
  const [editing, setEditing] = useState<Company | "new" | null>(null);
  const { limit, offset, q } = listState.params;
  const list = useCompanyList({ q, limit, offset });

  const columns = useMemo(
    () =>
      [
        createColumn<Company>({
          accessorKey: "name",
          labelKey: "contacts.companies.fields.name",
          enableHiding: false,
          cell: ({ row }) => (
            <div className="min-w-0">
              <Link
                href={`/companies/${row.original.uuid}`}
                className="block truncate font-medium underline-offset-2 hover:underline"
              >
                {row.original.name}
              </Link>
              <p className="text-muted-foreground truncate text-xs">
                {row.original.domain ?? "—"}
              </p>
            </div>
          ),
        }),
        createColumn<Company>({
          id: "health",
          accessorFn: (row) => row.health?.score ?? null,
          labelKey: "contacts.health.title",
          cell: ({ row }) =>
            row.original.health?.score != null ? (
              <CompanyHealthBadge health={row.original.health} />
            ) : (
              <span className="text-muted-foreground">—</span>
            ),
        }),
        createColumn<Company>({
          id: "external_id",
          accessorFn: (row) => row.external_id ?? "",
          labelKey: "contacts.companies.fields.external_id",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.external_id ? (
              <span className="font-mono text-xs">
                {row.original.external_id}
              </span>
            ) : (
              <span className="text-muted-foreground">—</span>
            ),
        }),
        createColumn<Company>({
          accessorKey: "tier",
          labelKey: "contacts.companies.fields.tier",
          cell: ({ row }) =>
            row.original.tier ? (
              <CompanyTier tier={row.original.tier} />
            ) : (
              <span className="text-muted-foreground">—</span>
            ),
        }),
        createColumn<Company>({
          accessorKey: "contact_count",
          labelKey: "contacts.companies.columns.contacts",
          meta: {
            headerClassName: "text-right",
            cellClassName: "text-right tabular-nums",
          },
          cell: ({ row }) => (
            <Link
              href={`/contacts?company=${row.original.uuid}`}
              className="underline-offset-2 hover:underline"
            >
              {row.original.contact_count}
            </Link>
          ),
        }),
        createColumn<Company>({
          accessorKey: "open_conversations",
          labelKey: "contacts.companies.columns.open",
          meta: {
            headerClassName: "text-right",
            cellClassName: "text-right tabular-nums",
          },
        }),
        createColumn<Company>({
          id: "mrr",
          accessorFn: (row) => row.revenue?.mrr_minor ?? 0,
          labelKey: "contacts.revenue.fields.mrr",
          meta: {
            headerClassName: "text-right",
            cellClassName: "text-right tabular-nums",
          },
          cell: ({ row }) => {
            const revenue = row.original.revenue;
            return revenue && revenue.mrr_minor > 0 ? (
              formatMinor(revenue.mrr_minor, revenue.currency, locale, {
                compact: true,
              })
            ) : (
              <span className="text-muted-foreground">—</span>
            );
          },
        }),
        createColumn<Company>({
          id: "account_owner",
          accessorFn: (row) => row.account_owner?.name ?? "",
          labelKey: "contacts.companies.fields.account_owner",
          cell: ({ row }) =>
            row.original.account_owner?.name ?? (
              <span className="text-muted-foreground">—</span>
            ),
        }),
      ] as ColumnDef<Company, unknown>[],
    [locale],
  );

  if (!scope.canRead) {
    return (
      <p className="text-muted-foreground p-8 text-sm">
        {t("contacts.no_permission")}
      </p>
    );
  }
  const hasFilters = Boolean(q);
  return (
    <div className="mx-auto flex w-full max-w-6xl flex-col gap-4">
      <PageHeader
        title={t("contacts.companies.title")}
        description={t("contacts.companies.description")}
        actions={
          scope.canWrite ? (
            <div className="flex items-center gap-2">
              <ContactsIOToolbar resource="companies" query={{ q: q ?? "" }} />
              <Button size="sm" onClick={() => setEditing("new")}>
                <Plus className="size-4" />
                {t("contacts.companies.new")}
              </Button>
            </div>
          ) : null
        }
      />
      <EntityTable
        columns={columns}
        data={list.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={(row) => router.push(`/companies/${row.uuid}`)}
        renderGridItem={(row) => <CompanyCard company={row} />}
        isLoading={list.isLoading}
        isError={list.isError}
        onRetry={() => void list.refetch()}
        emptyTitle={hasFilters ? t("table.no_results") : undefined}
        emptyDescription={
          hasFilters ? t("table.reset_filters") : t("contacts.companies.empty")
        }
        searchPlaceholder={t("contacts.companies.search")}
        total={list.data?.total ?? 0}
        state={listState.tableState}
        initialState={{ viewMode: "grid" }}
        features={{
          persistKey: "tenant-companies-v1",
          // The endpoint takes `q`, `limit` and `offset` only.
          sorting: false,
          columnFilters: false,
          rowSelection: false,
        }}
      />
      {editing && (
        <CompanyDialog
          key={editing === "new" ? "new" : editing.uuid}
          company={editing === "new" ? null : editing}
          onClose={() => setEditing(null)}
        />
      )}
    </div>
  );
}

function CompanyTier({ tier }: { tier: string }) {
  return (
    <span className="bg-signal-soft text-signal-text rounded-md px-2 py-0.5 text-[11px] font-medium">
      {tier}
    </span>
  );
}

/** Card of the grid view; the table core draws the border and padding. */
function CompanyCard({ company: c }: { company: Company }) {
  const { t, locale } = useLocale();
  return (
    <div className="flex flex-col gap-3 text-left">
      <div className="flex items-center gap-3">
        <span className="bg-muted flex size-9 items-center justify-center rounded-lg">
          <Building2 className="size-4" />
        </span>
        <div className="min-w-0">
          {/* Stretched link: the whole card opens the account screen (CF-85). */}
          <Link
            href={`/companies/${c.uuid}`}
            className="block truncate font-medium after:absolute after:inset-0 after:rounded-xl"
          >
            {c.name}
          </Link>
          <p className="text-muted-foreground truncate text-xs">
            {c.domain ?? "—"}
          </p>
        </div>
        <div className="ml-auto flex shrink-0 items-center gap-1">
          <CompanyHealthBadge health={c.health} />
          {c.tier && <CompanyTier tier={c.tier} />}
        </div>
      </div>
      <div className="text-muted-foreground flex items-center gap-4 text-xs">
        <Link
          href={`/contacts?company=${c.uuid}`}
          className="hover:text-foreground relative z-10 underline-offset-2 hover:underline"
        >
          {t("contacts.companies.contacts", { count: c.contact_count })}
        </Link>
        <span>
          {t("contacts.companies.open", { count: c.open_conversations })}
        </span>
        {c.revenue && c.revenue.mrr_minor > 0 && (
          <span className="text-foreground font-medium tabular-nums">
            {t("contacts.revenue.mrr_short", {
              amount: formatMinor(
                c.revenue.mrr_minor,
                c.revenue.currency,
                locale,
                { compact: true },
              ),
            })}
          </span>
        )}
        {c.account_owner && (
          <span className="ml-auto truncate">{c.account_owner.name}</span>
        )}
      </div>
    </div>
  );
}

export function CompanyDialog({
  company,
  onClose,
}: {
  company: Company | null;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const scope = useContactsScope();
  const m = useCompanyMutations();
  const [deleting, setDeleting] = useState(false);
  const [form, setForm] = useState({
    name: company?.name ?? "",
    domain: company?.domain ?? "",
    tier: company?.tier ?? "",
    external_id: company?.external_id ?? "",
    notes: company?.notes ?? "",
  });
  const [owner, setOwner] = useState<string | null>(
    company?.account_owner?.uuid ?? null,
  );
  const pending = m.create.isPending || m.update.isPending;
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>
            {company ? company.name : t("contacts.companies.new")}
          </DialogTitle>
          <DialogDescription>
            {t("contacts.companies.form_description")}
          </DialogDescription>
        </DialogHeader>
        <form
          className="grid gap-3"
          onSubmit={(e) => {
            e.preventDefault();
            const done = { onSuccess: onClose };
            const body = { ...form, account_owner: owner };
            if (company) m.update.mutate({ uuid: company.uuid, body }, done);
            else m.create.mutate(body, done);
          }}
        >
          {(["name", "domain", "external_id", "tier"] as const).map((k) => (
            <div key={k} className="grid gap-1.5">
              <Label htmlFor={`co-${k}`}>
                {t(`contacts.companies.fields.${k}`)}
              </Label>
              <Input
                id={`co-${k}`}
                value={form[k]}
                onChange={(e) =>
                  setForm((f) => ({ ...f, [k]: e.target.value }))
                }
              />
            </div>
          ))}
          <p className="text-muted-foreground -mt-1 text-xs">
            {t("contacts.companies.domain_hint")}{" "}
            {t("contacts.companies.external_id_hint")}
          </p>
          <div className="grid gap-1.5">
            <Label>{t("contacts.companies.fields.account_owner")}</Label>
            <OwnerPicker value={owner} onChange={setOwner} />
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="co-notes">{t("contacts.fields.notes")}</Label>
            <Textarea
              id="co-notes"
              rows={3}
              value={form.notes}
              onChange={(e) =>
                setForm((f) => ({ ...f, notes: e.target.value }))
              }
            />
          </div>
          <div className="flex items-center gap-2">
            {company && scope.canDelete && (
              <Button
                type="button"
                variant="ghost"
                size="sm"
                className="text-destructive"
                onClick={() => setDeleting(true)}
              >
                <Trash2 className="size-4" />
                {t("contacts.delete")}
              </Button>
            )}
            <Button
              type="submit"
              className="ml-auto"
              disabled={pending || !form.name.trim()}
            >
              {company ? t("contacts.save") : t("contacts.create")}
            </Button>
          </div>
        </form>
        {company && <CompanyHealthCard companyUuid={company.uuid} />}
        {company && <CompanyRevenueCard companyUuid={company.uuid} />}
        {company && (
          <ConfirmDialog
            open={deleting}
            title={t("contacts.companies.delete_title")}
            description={t("contacts.companies.delete_description")}
            variant="destructive"
            isPending={m.remove.isPending}
            onCancel={() => setDeleting(false)}
            onConfirm={() =>
              m.remove.mutate(company.uuid, { onSuccess: onClose })
            }
          />
        )}
      </DialogContent>
    </Dialog>
  );
}
