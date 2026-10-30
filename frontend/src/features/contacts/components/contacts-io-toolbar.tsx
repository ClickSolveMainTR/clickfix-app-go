"use client";

import { useEffect, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Download, FileDown, Undo2, Upload } from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { triggerBrowserDownload } from "@/lib/api/platform-form-request";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

import { contactsKeys, useContactsScope } from "../hooks/use-contacts";
import {
  contactsIOService,
  type ContactImportJob,
  type ContactIOFormat,
  type ContactIOResource,
} from "../services/contacts-io.service";

const SKIP = "__skip__";
const ROLLBACKABLE = new Set(["applied"]);
const RUNNING = new Set(["queued", "applying"]);

/** Export / import menu for the contacts and companies pages (CF-84). */
export function ContactsIOToolbar({
  resource,
  query,
}: {
  resource: ContactIOResource;
  query?: { q?: string; company?: string };
}) {
  const { t } = useLocale();
  const { org, canWrite } = useContactsScope();
  const [importing, setImporting] = useState(false);
  const [busy, setBusy] = useState(false);
  if (!canWrite) return null;

  const download = async (
    kind: "export" | "sample",
    format: ContactIOFormat,
  ) => {
    setBusy(true);
    try {
      const { blob, filename } =
        kind === "export"
          ? await contactsIOService.export(org, resource, format, query)
          : await contactsIOService.sample(org, resource, format);
      triggerBrowserDownload(blob, filename ?? `${resource}.${format}`);
    } catch {
      appToast.error(t("contacts.io.download_failed"));
    } finally {
      setBusy(false);
    }
  };

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button size="sm" variant="outline" disabled={busy}>
            <FileDown className="size-4" />
            {t("contacts.io.menu")}
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuLabel>{t("contacts.io.export")}</DropdownMenuLabel>
          <DropdownMenuItem onSelect={() => void download("export", "xlsx")}>
            <Download className="size-4" />
            Excel (.xlsx)
          </DropdownMenuItem>
          <DropdownMenuItem onSelect={() => void download("export", "csv")}>
            <Download className="size-4" />
            CSV
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem onSelect={() => setImporting(true)}>
            <Upload className="size-4" />
            {t("contacts.io.import")}
          </DropdownMenuItem>
          <DropdownMenuItem onSelect={() => void download("sample", "xlsx")}>
            <FileDown className="size-4" />
            {t("contacts.io.sample")}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      {importing && (
        <ImportDialog resource={resource} onClose={() => setImporting(false)} />
      )}
    </>
  );
}

function ImportDialog({
  resource,
  onClose,
}: {
  resource: ContactIOResource;
  onClose: () => void;
}) {
  const { t, locale } = useLocale();
  const { org } = useContactsScope();
  const qc = useQueryClient();
  const [file, setFile] = useState<File | null>(null);
  const [job, setJob] = useState<ContactImportJob | null>(null);
  const [mapping, setMapping] = useState<Record<string, string>>({});
  const [pending, setPending] = useState(false);
  const recentKey = [...contactsKeys.all(org), "imports", resource];
  const recent = useQuery({
    queryKey: recentKey,
    queryFn: () => contactsIOService.list(org, resource),
    enabled: Boolean(org) && !job,
  });
  // Poll the job while the worker applies it.
  const live = useQuery({
    queryKey: [...contactsKeys.all(org), "import", job?.uuid],
    queryFn: () => contactsIOService.get(org, job!.uuid),
    enabled: Boolean(job && RUNNING.has(job.status)),
    refetchInterval: (q) =>
      q.state.data && !RUNNING.has(q.state.data.status) ? false : 1500,
  });
  const current =
    live.data && live.data.uuid === job?.uuid ? live.data : (job ?? null);
  const liveStatus = live.data?.status;
  useEffect(() => {
    if (liveStatus === "applied") {
      void qc.invalidateQueries({ queryKey: contactsKeys.all(org) });
    }
  }, [liveStatus, org, qc]);

  const run = async (fn: () => Promise<ContactImportJob>) => {
    setPending(true);
    try {
      const next = await fn();
      setJob(next);
      return next;
    } catch {
      return null;
    } finally {
      setPending(false);
    }
  };

  const upload = async () => {
    if (!file) return;
    const next = await run(() =>
      contactsIOService.upload(org, resource, file, locale),
    );
    if (next) setMapping(next.mapping ?? {});
  };

  const fieldLabel = (key: string) => t(`contacts.io.field.${resource}.${key}`);
  const headers = Object.keys(job?.mapping ?? {});
  const summary = current?.preview_summary;
  const step = !job
    ? "upload"
    : job.status === "uploaded" || job.status === "mapped"
      ? "mapping"
      : job.status === "previewed"
        ? "preview"
        : "done";

  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-w-xl">
        <DialogHeader>
          <DialogTitle>{t(`contacts.io.title_${resource}`)}</DialogTitle>
          <DialogDescription>
            {t(`contacts.io.description_${resource}`)}
          </DialogDescription>
        </DialogHeader>

        {step === "upload" && (
          <div className="grid gap-4">
            <Input
              type="file"
              accept=".csv,.tsv,.xlsx"
              onChange={(e) => setFile(e.target.files?.[0] ?? null)}
            />
            {(recent.data?.items.length ?? 0) > 0 && (
              <div className="grid gap-2">
                <p className="text-muted-foreground text-xs font-medium">
                  {t("contacts.io.recent")}
                </p>
                {recent.data!.items.map((j) => (
                  <RecentImport
                    key={j.uuid}
                    job={j}
                    onRolledBack={() => {
                      void qc.invalidateQueries({
                        queryKey: contactsKeys.all(org),
                      });
                    }}
                  />
                ))}
              </div>
            )}
          </div>
        )}

        {step === "mapping" && (
          <div className="grid gap-2">
            <p className="text-muted-foreground text-sm">
              {t("contacts.io.mapping_hint")}
            </p>
            {headers.map((h) => (
              <div key={h} className="grid grid-cols-2 items-center gap-3">
                <span className="truncate font-mono text-xs">{h}</span>
                <Select
                  value={mapping[h] || SKIP}
                  onValueChange={(v) =>
                    setMapping((m) => ({ ...m, [h]: v === SKIP ? "" : v }))
                  }
                >
                  <SelectTrigger size="sm">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value={SKIP}>
                      {t("contacts.io.skip")}
                    </SelectItem>
                    {job!.fields.map((f) => (
                      <SelectItem key={f.key} value={f.key}>
                        {fieldLabel(f.key)}
                        {f.required ? " *" : ""}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            ))}
          </div>
        )}

        {(step === "preview" || step === "done") && summary && (
          <div className="grid gap-3">
            <div className="grid grid-cols-3 gap-2 text-center">
              <Stat label={t("contacts.io.creates")} value={summary.creates} />
              <Stat label={t("contacts.io.updates")} value={summary.updates} />
              <Stat
                label={t("contacts.io.invalid")}
                value={summary.invalid}
                tone={summary.invalid ? "destructive" : undefined}
              />
            </div>
            {(summary.errors?.length ?? 0) > 0 && (
              <ul className="bg-destructive/5 max-h-40 overflow-y-auto rounded-md p-2 text-xs">
                {summary.errors!.map((e) => (
                  <li key={`${e.index}-${e.error}`}>
                    {t("contacts.io.row", { index: e.index })}: {e.error}
                  </li>
                ))}
              </ul>
            )}
            {step === "preview" && (
              <p className="text-muted-foreground text-xs">
                {t("contacts.io.dedupe_hint")}
              </p>
            )}
            {step === "done" && current && (
              <p className="text-sm">
                {t(`contacts.io.status.${current.status}`)}
                {current.error ? ` — ${current.error}` : ""}
              </p>
            )}
          </div>
        )}

        <DialogFooter>
          {step === "upload" && (
            <Button disabled={!file || pending} onClick={() => void upload()}>
              {t("contacts.io.upload")}
            </Button>
          )}
          {step === "mapping" && (
            <Button
              disabled={pending}
              onClick={() =>
                void run(async () => {
                  await contactsIOService.updateMapping(
                    org,
                    job!.uuid,
                    mapping,
                    {},
                  );
                  return contactsIOService.preview(org, job!.uuid);
                })
              }
            >
              {t("contacts.io.preview")}
            </Button>
          )}
          {step === "preview" && (
            <>
              <Button
                variant="ghost"
                onClick={() =>
                  setJob((j) => (j ? { ...j, status: "mapped" } : j))
                }
              >
                {t("contacts.back")}
              </Button>
              <Button
                disabled={pending || !summary?.valid}
                onClick={() =>
                  void run(() => contactsIOService.confirm(org, job!.uuid))
                }
              >
                {t("contacts.io.confirm", { count: summary?.valid ?? 0 })}
              </Button>
            </>
          )}
          {step === "done" && (
            <Button
              disabled={RUNNING.has(current?.status ?? "")}
              onClick={onClose}
            >
              {t("contacts.io.close")}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function Stat({
  label,
  value,
  tone,
}: {
  label: string;
  value?: number;
  tone?: "destructive";
}) {
  return (
    <div className="rounded-lg border p-2">
      <p
        className={
          tone === "destructive"
            ? "text-destructive text-lg font-semibold tabular-nums"
            : "text-lg font-semibold tabular-nums"
        }
      >
        {value ?? 0}
      </p>
      <p className="text-muted-foreground text-xs">{label}</p>
    </div>
  );
}

function RecentImport({
  job,
  onRolledBack,
}: {
  job: ContactImportJob;
  onRolledBack: () => void;
}) {
  const { t, locale } = useLocale();
  const { org } = useContactsScope();
  const qc = useQueryClient();
  const [pending, setPending] = useState(false);
  const [now] = useState(() => Date.now());
  const canRollback =
    ROLLBACKABLE.has(job.status) &&
    job.rollback_until &&
    new Date(job.rollback_until).getTime() > now;
  return (
    <div className="flex items-center gap-2 rounded-md border px-3 py-2 text-xs">
      <span className="tabular-nums">
        {new Date(job.created_at).toLocaleString(locale)}
      </span>
      <span className="text-muted-foreground">
        {t(`contacts.io.status.${job.status}`)}
      </span>
      {canRollback && (
        <Button
          size="sm"
          variant="ghost"
          className="ml-auto h-7"
          disabled={pending}
          onClick={async () => {
            setPending(true);
            try {
              await contactsIOService.rollback(org, job.uuid);
              appToast.success(t("contacts.io.rolled_back"));
              void qc.invalidateQueries({
                queryKey: [...contactsKeys.all(org), "imports"],
              });
              onRolledBack();
            } catch {
              // unwrap already toasted
            } finally {
              setPending(false);
            }
          }}
        >
          <Undo2 className="size-3.5" />
          {t("contacts.io.rollback")}
        </Button>
      )}
    </div>
  );
}
