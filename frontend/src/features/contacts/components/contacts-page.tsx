"use client";

import { CallButton } from "@/features/softphone/softphone";
import Link from "next/link";
import { useCallback, useMemo, useState } from "react";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import type { ColumnDef } from "@tanstack/react-table";
import {
  Building2,
  GitMerge,
  Plus,
  ShieldCheck,
  Trash2,
  UserRound,
} from "lucide-react";

import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import { EntityTable, useServerListState } from "@/components/entity";
import { PageHeader } from "@/components/layout/page-header";
import { createColumn } from "@/components/tables";
import { Button } from "@/components/ui/button";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { Skeleton } from "@/components/ui/skeleton";
import { Textarea } from "@/components/ui/textarea";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { ChannelGlyph } from "@/features/inbox/components/channel-glyph";
import { initials, shortAgo } from "@/features/inbox/lib/format";
import { datetime } from "@/lib/utils";
import { SidebarAppsPanel } from "@/features/sidebar-apps/components/sidebar-apps-panel";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

import {
  useCompanyList,
  useContact,
  useContactList,
  useContactMutations,
  useContactsScope,
} from "../hooks/use-contacts";
import type { Contact, ContactSort } from "../services/contacts.service";

import { ContactsIOToolbar } from "./contacts-io-toolbar";
import { CustomFieldValuesList } from "./customer-fields";
import { IdentityBadge } from "./identity-badge";

const SORTS: readonly ContactSort[] = [
  "created_at",
  "-created_at",
  "name",
  "-name",
  "last_seen_at",
  "-last_seen_at",
];

/** Contacts: who writes to the organization, on which channels (CF-84). */
export function ContactsPage() {
  const { t, locale } = useLocale();
  const router = useRouter();
  const pathname = usePathname();
  const params = useSearchParams();
  const scope = useContactsScope();
  const listState = useServerListState({
    initialSort: "-created_at",
    initialPageSize: 50,
    searchDebounceMs: 250,
  });
  const [creating, setCreating] = useState(false);
  const selected = params.get("c");
  const company = params.get("company") ?? undefined;

  const { limit, offset, q } = listState.params;
  // The endpoint sorts by one field; anything else falls back to its default.
  const sort = SORTS.find((item) => item === listState.params.sort);
  const list = useContactList({ q, company, limit, offset, sort });
  const select = useCallback(
    (uuid: string | null) => {
      const next = new URLSearchParams(params.toString());
      if (uuid) next.set("c", uuid);
      else next.delete("c");
      router.replace(`${pathname}?${next.toString()}`, { scroll: false });
    },
    [params, pathname, router],
  );

  const columns = useMemo(
    () =>
      [
        createColumn<Contact>({
          id: "name",
          accessorFn: (row) => row.name || row.email || row.phone || "",
          labelKey: "contacts.columns.name",
          gridPrimary: true,
          enableHiding: false,
          cell: ({ row }) => {
            const c = row.original;
            return (
              <div className="flex items-center gap-2.5">
                <span className="bg-muted flex size-8 shrink-0 items-center justify-center rounded-full text-[11px] font-semibold">
                  {initials(c.name || c.email)}
                </span>
                <div className="min-w-0">
                  <p className="flex min-w-0 items-center gap-1.5 font-medium">
                    <span className="truncate">
                      {c.name || c.email || c.phone || "—"}
                    </span>
                    <IdentityBadge state={c.identity_state} compact />
                  </p>
                  {c.job_title && (
                    <p className="text-muted-foreground truncate text-xs font-normal">
                      {c.job_title}
                    </p>
                  )}
                </div>
              </div>
            );
          },
        }),
        createColumn<Contact>({
          id: "contact",
          accessorFn: (row) => row.email ?? "",
          labelKey: "contacts.columns.contact",
          enableSorting: false,
          cell: ({ row }) => (
            <div className="text-muted-foreground min-w-0">
              <p className="truncate">{row.original.email ?? "—"}</p>
              {row.original.phone && (
                <p className="truncate text-xs">{row.original.phone}</p>
              )}
            </div>
          ),
        }),
        createColumn<Contact>({
          id: "external_id",
          accessorFn: (row) => row.external_id ?? "",
          labelKey: "contacts.columns.external_id",
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
        createColumn<Contact>({
          id: "company",
          accessorFn: (row) => row.company?.name ?? "",
          labelKey: "contacts.columns.company",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.company?.name ?? (
              <span className="text-muted-foreground">—</span>
            ),
        }),
        createColumn<Contact>({
          id: "channels",
          accessorFn: (row) => row.channel_types.join(","),
          labelKey: "contacts.columns.channels",
          enableSorting: false,
          cell: ({ row }) => (
            <div className="flex gap-1">
              {row.original.channel_types.map((ch) => (
                <ChannelGlyph
                  key={ch}
                  type={ch}
                  className="size-4"
                  title={t(`channels.types.${ch}`)}
                />
              ))}
            </div>
          ),
        }),
        createColumn<Contact>({
          accessorKey: "conversation_count",
          labelKey: "contacts.columns.conversations",
          enableSorting: false,
          meta: {
            headerClassName: "text-right",
            cellClassName: "text-right tabular-nums",
          },
        }),
        createColumn<Contact>({
          accessorKey: "last_seen_at",
          labelKey: "contacts.columns.last_seen",
          meta: {
            headerClassName: "text-right",
            cellClassName: "text-muted-foreground text-right text-xs",
          },
          cell: ({ row }) =>
            row.original.last_seen_at
              ? shortAgo(row.original.last_seen_at, locale)
              : "—",
        }),
      ] as ColumnDef<Contact, unknown>[],
    [locale, t],
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
        title={t("contacts.title")}
        description={t("contacts.description")}
        actions={
          scope.canWrite ? (
            <div className="flex items-center gap-2">
              <ContactsIOToolbar
                resource="contacts"
                query={{ q: q ?? "", company }}
              />
              <Button size="sm" onClick={() => setCreating(true)}>
                <Plus className="size-4" />
                {t("contacts.new")}
              </Button>
            </div>
          ) : null
        }
      />
      <EntityTable
        columns={columns}
        data={list.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={(row) => select(row.uuid)}
        isLoading={list.isLoading}
        isError={list.isError}
        onRetry={() => void list.refetch()}
        emptyTitle={hasFilters ? t("table.no_results") : undefined}
        emptyDescription={
          hasFilters ? t("table.reset_filters") : t("contacts.empty")
        }
        searchPlaceholder={t("contacts.search")}
        total={list.data?.total ?? 0}
        state={listState.tableState}
        features={{
          persistKey: "tenant-contacts-v1",
          // The endpoint takes `q`, `company` and a single `sort` field.
          multiSort: false,
          columnFilters: false,
          rowSelection: false,
        }}
      />
      <ContactSheet uuid={selected} onClose={() => select(null)} />
      <CreateContactDialog
        open={creating}
        onOpenChange={setCreating}
        onCreated={(uuid) => select(uuid)}
      />
    </div>
  );
}

function ContactSheet({
  uuid,
  onClose,
}: {
  uuid: string | null;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const { can } = usePermission();
  const contact = useContact(uuid);
  return (
    <Sheet open={Boolean(uuid)} onOpenChange={(o) => !o && onClose()}>
      <SheetContent className="w-full overflow-y-auto sm:max-w-md">
        <SheetHeader>
          <SheetTitle>
            {contact.data?.name || contact.data?.email || t("contacts.detail")}
          </SheetTitle>
          <CallButton phone={contact.data?.phone} />
          <SheetDescription>
            {t("contacts.detail_description")}
          </SheetDescription>
          {uuid && can(permissions.tenant.settings.write) ? (
            <Button variant="outline" size="sm" className="w-fit" asChild>
              <Link
                href={`${routes.cms.settings.privacy}?tab=consents&contact=${uuid}`}
              >
                <ShieldCheck className="size-4" />
                {t("contacts.privacy_link")}
              </Link>
            </Button>
          ) : null}
        </SheetHeader>
        {contact.data ? (
          <ContactForm
            key={contact.data.uuid + contact.data.updated_at}
            contact={contact.data}
            onDeleted={onClose}
          />
        ) : (
          <Skeleton className="m-4 h-40" />
        )}
        {uuid && (
          <div className="px-4 pb-4">
            <SidebarAppsPanel placement="contact_panel" contactUuid={uuid} />
          </div>
        )}
      </SheetContent>
    </Sheet>
  );
}

function ContactForm({
  contact,
  onDeleted,
}: {
  contact: Contact;
  onDeleted: () => void;
}) {
  const { t } = useLocale();
  const scope = useContactsScope();
  const m = useContactMutations();
  const [form, setForm] = useState({
    name: contact.name,
    email: contact.email ?? "",
    phone: contact.phone ?? "",
    job_title: contact.job_title,
    notes: contact.notes,
    external_id: contact.external_id ?? "",
  });
  const [company, setCompany] = useState(contact.company);
  const [merging, setMerging] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const dirty =
    form.name !== contact.name ||
    form.email !== (contact.email ?? "") ||
    form.phone !== (contact.phone ?? "") ||
    form.job_title !== contact.job_title ||
    form.notes !== contact.notes ||
    form.external_id !== (contact.external_id ?? "") ||
    company?.uuid !== contact.company?.uuid;
  const field = (key: keyof typeof form, label: string, type = "text") => (
    <div className="grid gap-1.5">
      <Label htmlFor={`contact-${key}`}>{label}</Label>
      <Input
        id={`contact-${key}`}
        type={type}
        value={form[key]}
        disabled={!scope.canWrite}
        onChange={(e) => setForm((f) => ({ ...f, [key]: e.target.value }))}
      />
    </div>
  );
  return (
    <div className="flex flex-col gap-5 px-4 pb-6">
      <div className="flex flex-wrap items-center gap-2">
        <IdentityBadge state={contact.identity_state} />
        {contact.verified_at && (
          <span className="text-muted-foreground text-xs">
            {t("contacts.identity.verified_at", {
              date: datetime(contact.verified_at, "dd.MM.yyyy HH:mm"),
            })}
          </span>
        )}
      </div>
      <div className="grid gap-3">
        {field("name", t("contacts.fields.name"))}
        {field("email", t("contacts.fields.email"), "email")}
        {field("phone", t("contacts.fields.phone"), "tel")}
        {field("job_title", t("contacts.fields.job_title"))}
        {field("external_id", t("contacts.fields.external_id"))}
        <p className="text-muted-foreground -mt-1.5 text-xs">
          {t("contacts.external_id.hint")}
        </p>
        <div className="grid gap-1.5">
          <Label>{t("contacts.fields.company")}</Label>
          <CompanyPicker
            value={company}
            onChange={setCompany}
            disabled={!scope.canWrite}
          />
        </div>
        <div className="grid gap-1.5">
          <Label htmlFor="contact-notes">{t("contacts.fields.notes")}</Label>
          <Textarea
            id="contact-notes"
            rows={3}
            value={form.notes}
            disabled={!scope.canWrite}
            onChange={(e) => setForm((f) => ({ ...f, notes: e.target.value }))}
          />
        </div>
        {scope.canWrite && (
          <Button
            disabled={!dirty || m.update.isPending}
            onClick={() =>
              m.update.mutate({
                uuid: contact.uuid,
                body: { ...form, company: company?.uuid ?? null },
              })
            }
          >
            {t("contacts.save")}
          </Button>
        )}
      </div>

      <CustomFieldValuesList
        entityType="contact"
        values={contact.custom_fields as Record<string, unknown>}
        title={t("contacts.custom_fields.contact")}
      />

      <section>
        <h3 className="text-muted-foreground mb-2 text-[11px] font-semibold tracking-wide uppercase">
          {t("contacts.identities")}
        </h3>
        <ul className="flex flex-col gap-1.5">
          {(contact.identities ?? []).map((i) => (
            <li
              key={i.id}
              className="bg-muted/40 flex items-center gap-2 rounded-md px-2.5 py-1.5 text-xs"
            >
              <ChannelGlyph type={i.channel_type} className="size-3.5" />
              <span className="font-medium">
                {t(`channels.types.${i.channel_type}`)}
              </span>
              <span className="text-muted-foreground truncate">
                {i.username || i.display_name || i.external_id}
              </span>
            </li>
          ))}
          {(contact.identities ?? []).length === 0 && (
            <li className="text-muted-foreground text-xs">
              {t("contacts.no_identities")}
            </li>
          )}
        </ul>
      </section>

      <div className="flex flex-wrap gap-2 border-t pt-4">
        {scope.canWrite && (
          <Button variant="outline" size="sm" onClick={() => setMerging(true)}>
            <GitMerge className="size-4" />
            {t("contacts.merge")}
          </Button>
        )}
        {scope.canDelete && (
          <Button
            variant="ghost"
            size="sm"
            className="text-destructive"
            onClick={() => setDeleting(true)}
          >
            <Trash2 className="size-4" />
            {t("contacts.delete")}
          </Button>
        )}
      </div>
      <MergeDialog open={merging} onOpenChange={setMerging} into={contact} />
      <ConfirmDialog
        open={deleting}
        title={t("contacts.delete_title")}
        description={t("contacts.delete_description")}
        variant="destructive"
        isPending={m.remove.isPending}
        onCancel={() => setDeleting(false)}
        onConfirm={() =>
          m.remove.mutate(contact.uuid, {
            onSuccess: () => {
              setDeleting(false);
              onDeleted();
            },
          })
        }
      />
    </div>
  );
}

function CompanyPicker({
  value,
  onChange,
  disabled,
}: {
  value: { uuid: string; name: string } | null;
  onChange: (v: { uuid: string; name: string } | null) => void;
  disabled?: boolean;
}) {
  const { t } = useLocale();
  const [open, setOpen] = useState(false);
  const [q, setQ] = useState("");
  const companies = useCompanyList({ q, limit: 20, offset: 0 });
  if (!open) {
    return (
      <Button
        variant="outline"
        className="justify-start font-normal"
        disabled={disabled}
        onClick={() => setOpen(true)}
      >
        <Building2 className="size-4" />
        {value?.name ?? (
          <span className="text-muted-foreground">
            {t("contacts.no_company")}
          </span>
        )}
      </Button>
    );
  }
  return (
    <Command shouldFilter={false} className="rounded-md border">
      <CommandInput
        value={q}
        onValueChange={setQ}
        placeholder={t("contacts.search_company")}
        autoFocus
      />
      <CommandList>
        <CommandEmpty>{t("contacts.no_match")}</CommandEmpty>
        <CommandGroup>
          <CommandItem
            onSelect={() => {
              onChange(null);
              setOpen(false);
            }}
          >
            {t("contacts.no_company")}
          </CommandItem>
          {(companies.data?.items ?? []).map((c) => (
            <CommandItem
              key={c.uuid}
              value={c.uuid}
              onSelect={() => {
                onChange({ uuid: c.uuid, name: c.name });
                setOpen(false);
              }}
            >
              <Building2 className="size-4" />
              {c.name}
              {c.domain && (
                <span className="text-muted-foreground ml-auto text-xs">
                  {c.domain}
                </span>
              )}
            </CommandItem>
          ))}
        </CommandGroup>
      </CommandList>
    </Command>
  );
}

function MergeDialog({
  open,
  onOpenChange,
  into,
}: {
  open: boolean;
  onOpenChange: (o: boolean) => void;
  into: Contact;
}) {
  const { t } = useLocale();
  const [q, setQ] = useState("");
  const [from, setFrom] = useState<Contact | null>(null);
  const list = useContactList({ q, limit: 10, offset: 0 });
  const m = useContactMutations();
  const candidates = (list.data?.items ?? []).filter(
    (c) => c.uuid !== into.uuid,
  );
  return (
    <Dialog
      open={open}
      onOpenChange={(o) => {
        onOpenChange(o);
        if (!o) setFrom(null);
      }}
    >
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>{t("contacts.merge_title")}</DialogTitle>
          <DialogDescription>
            {t("contacts.merge_description", {
              name: into.name || into.email || "",
            })}
          </DialogDescription>
        </DialogHeader>
        {from ? (
          <div className="flex flex-col gap-4">
            <p className="text-sm">
              {t("contacts.merge_confirm", {
                from: from.name || from.email || "",
                into: into.name || into.email || "",
              })}
            </p>
            <DialogFooter>
              <Button variant="outline" onClick={() => setFrom(null)}>
                {t("contacts.back")}
              </Button>
              <Button
                disabled={m.merge.isPending}
                onClick={() =>
                  m.merge.mutate(
                    { into: into.uuid, from: from.uuid },
                    {
                      onSuccess: () => {
                        setFrom(null);
                        onOpenChange(false);
                      },
                    },
                  )
                }
              >
                <GitMerge className="size-4" />
                {t("contacts.merge")}
              </Button>
            </DialogFooter>
          </div>
        ) : (
          <Command shouldFilter={false} className="rounded-md border">
            <CommandInput
              value={q}
              onValueChange={setQ}
              placeholder={t("contacts.search")}
              autoFocus
            />
            <CommandList>
              <CommandEmpty>{t("contacts.no_match")}</CommandEmpty>
              <CommandGroup>
                {candidates.map((c) => (
                  <CommandItem
                    key={c.uuid}
                    value={c.uuid}
                    onSelect={() => setFrom(c)}
                  >
                    <UserRound className="size-4" />
                    <span className="truncate">
                      {c.name || c.email || c.phone}
                    </span>
                    <span className="text-muted-foreground ml-auto truncate text-xs">
                      {c.email ?? c.phone ?? ""}
                    </span>
                  </CommandItem>
                ))}
              </CommandGroup>
            </CommandList>
          </Command>
        )}
      </DialogContent>
    </Dialog>
  );
}

function CreateContactDialog({
  open,
  onOpenChange,
  onCreated,
}: {
  open: boolean;
  onOpenChange: (o: boolean) => void;
  onCreated: (uuid: string) => void;
}) {
  const { t } = useLocale();
  const m = useContactMutations();
  const [form, setForm] = useState({ name: "", email: "", phone: "" });
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-sm">
        <DialogHeader>
          <DialogTitle>{t("contacts.new")}</DialogTitle>
          <DialogDescription>{t("contacts.new_description")}</DialogDescription>
        </DialogHeader>
        <form
          className="grid gap-3"
          onSubmit={(e) => {
            e.preventDefault();
            m.create.mutate(form, {
              onSuccess: (c) => {
                onOpenChange(false);
                setForm({ name: "", email: "", phone: "" });
                onCreated(c.uuid);
              },
            });
          }}
        >
          {(["name", "email", "phone"] as const).map((k) => (
            <div key={k} className="grid gap-1.5">
              <Label htmlFor={`new-${k}`}>{t(`contacts.fields.${k}`)}</Label>
              <Input
                id={`new-${k}`}
                type={k === "email" ? "email" : k === "phone" ? "tel" : "text"}
                value={form[k]}
                onChange={(e) =>
                  setForm((f) => ({ ...f, [k]: e.target.value }))
                }
              />
            </div>
          ))}
          <DialogFooter>
            <Button
              type="submit"
              disabled={
                m.create.isPending || !(form.name || form.email || form.phone)
              }
            >
              {t("contacts.create")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
