"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { Building2, Plus, Search, Trash2 } from "lucide-react";

import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import { PageHeader } from "@/components/layout/page-header";
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
import { Skeleton } from "@/components/ui/skeleton";
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
  const scope = useContactsScope();
  const [q, setQ] = useState("");
  const [debounced, setDebounced] = useState("");
  const [editing, setEditing] = useState<Company | "new" | null>(null);
  useEffect(() => {
    const id = setTimeout(() => setDebounced(q.trim()), 250);
    return () => clearTimeout(id);
  }, [q]);
  const list = useCompanyList({ q: debounced, limit: 100, offset: 0 });

  if (!scope.canRead) {
    return (
      <p className="text-muted-foreground p-8 text-sm">
        {t("contacts.no_permission")}
      </p>
    );
  }
  const items = list.data?.items ?? [];
  return (
    <div className="mx-auto flex w-full max-w-6xl flex-col gap-4">
      <PageHeader
        title={t("contacts.companies.title")}
        description={t("contacts.companies.description")}
        actions={
          scope.canWrite ? (
            <div className="flex items-center gap-2">
              <ContactsIOToolbar
                resource="companies"
                query={{ q: debounced }}
              />
              <Button size="sm" onClick={() => setEditing("new")}>
                <Plus className="size-4" />
                {t("contacts.companies.new")}
              </Button>
            </div>
          ) : null
        }
      />
      <div className="relative max-w-sm">
        <Search className="text-muted-foreground absolute top-1/2 left-2.5 size-4 -translate-y-1/2" />
        <Input
          value={q}
          onChange={(e) => setQ(e.target.value)}
          placeholder={t("contacts.companies.search")}
          className="pl-8"
        />
      </div>
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
        {list.isLoading &&
          Array.from({ length: 6 }).map((_, i) => (
            <Skeleton key={i} className="h-28 rounded-xl" />
          ))}
        {!list.isLoading && items.length === 0 && (
          <p className="text-muted-foreground col-span-full py-12 text-center text-sm">
            {t("contacts.companies.empty")}
          </p>
        )}
        {items.map((c) => (
          <div
            key={c.uuid}
            className="bg-card hover:border-foreground/20 relative flex flex-col gap-3 rounded-xl border p-4 text-left transition-colors"
          >
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
                {c.tier && (
                  <span className="bg-signal-soft text-signal-text rounded-md px-2 py-0.5 text-[11px] font-medium">
                    {c.tier}
                  </span>
                )}
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
        ))}
      </div>
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
          {(["name", "domain", "tier"] as const).map((k) => (
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
            {t("contacts.companies.domain_hint")}
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
