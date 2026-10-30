"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  BookOpen,
  Blocks,
  Copy,
  Eye,
  Pencil,
  Plus,
  Trash2,
} from "lucide-react";
import { createElement, useState } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import { AppDialog } from "@/components/dialogs/app-dialog";
import { FormActions } from "@/components/forms/form-actions";
import { PageHeader } from "@/components/layout/page-header";
import { BrandLogo } from "@/components/brand-logos";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { Permission } from "@/config/permissions";
import { useActiveOrganization } from "@/features/organizations";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

import { sidebarAppKeys, useSidebarApps } from "../hooks/use-sidebar-apps";
import {
  appBrand,
  GALLERY,
  galleryBrand,
  galleryInput,
  hasPlaceholder,
  type GalleryApp,
} from "../lib/gallery";
import {
  APP_ICONS,
  SCOPES,
  SCOPE_PERMISSION,
  appIcon,
  validAppUrl,
} from "../lib/protocol";
import {
  sidebarAppsService,
  type SidebarApp,
  type SidebarAppInput,
  type SidebarAppPlacement,
  type SidebarAppScope,
} from "../services/sidebar-apps.service";

const PLACEMENTS: SidebarAppPlacement[] = [
  "conversation_sidebar",
  "contact_panel",
  "nav",
];

/** The context scope a placement cannot launch without. */
const REQUIRED_SCOPE: Partial<Record<SidebarAppPlacement, SidebarAppScope>> = {
  conversation_sidebar: "conversation.read",
  contact_panel: "contact.read",
};

const scopeKey = (s: string) => s.replace(".", "_");

/** Developer guide "Kendi uygulamanızı yapın" on the public docs site. */
export const APPS_GUIDE_PATH = "/docs/apps";

type Editing =
  | { app: SidebarApp }
  | { template: GalleryApp | null; initial: SidebarAppInput };

function copy(value: string, done: string) {
  void navigator.clipboard.writeText(value).then(() => appToast.success(done));
}

/**
 * Sidebar app registry (CF-118). Owners/admins with tenant.settings.write
 * register HTTPS iframe apps, choose where they appear and which scopes the
 * context token may carry. Every edit bumps the revision, revoking tokens
 * issued for the previous registration.
 */
export function SidebarAppsSettingsPage() {
  const { t } = useLocale();
  const qc = useQueryClient();
  const { active: org } = useActiveOrganization();
  const perms = org?.permissions ?? [];
  const canManage = perms.includes(Permission.TenantSettingsWrite);
  const canRead = perms.includes(Permission.TenantConversationsRead);
  const apps = useSidebarApps();
  const [editing, setEditing] = useState<Editing | null>(null);
  const [deleting, setDeleting] = useState<SidebarApp | null>(null);
  const key = sidebarAppKeys.all(org?.slug);

  const fail = (error: unknown) =>
    appToast.error(
      isApiError(error) && error.status === 403
        ? t("apps.toast.owner_only")
        : isApiError(error) && error.status === 409
          ? t("apps.toast.conflict")
          : isApiError(error)
            ? error.message
            : t("apps.toast.failed"),
    );
  const save = useMutation({
    mutationFn: (body: SidebarAppInput) =>
      editing && "app" in editing
        ? sidebarAppsService.update(editing.app.uuid, body)
        : sidebarAppsService.create(body),
    onSuccess: async () => {
      setEditing(null);
      await qc.invalidateQueries({ queryKey: key });
      appToast.success(t("apps.toast.saved"));
    },
    onError: fail,
  });
  const toggle = useMutation({
    mutationFn: (app: SidebarApp) =>
      sidebarAppsService.update(app.uuid, {
        name: app.name,
        url: app.url,
        icon: app.icon,
        placement: app.placement,
        scopes: app.scopes,
        enabled: !app.enabled,
        revision: app.revision,
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: key }),
    onError: fail,
  });
  const remove = useMutation({
    mutationFn: (uuid: string) => sidebarAppsService.remove(uuid),
    onSuccess: async () => {
      setDeleting(null);
      await qc.invalidateQueries({ queryKey: key });
      appToast.success(t("apps.toast.deleted"));
    },
    onError: (error) => {
      setDeleting(null);
      fail(error);
    },
  });
  const items = apps.data ?? [];
  const startNew = (template: GalleryApp | null) =>
    setEditing({
      template,
      initial: template
        ? galleryInput(template, t(`apps.gallery.${template.id}.name`))
        : {
            name: "",
            url: "https://",
            icon: "puzzle",
            placement: "conversation_sidebar",
            scopes: ["conversation.read"],
            enabled: true,
          },
    });

  return (
    <div className="space-y-6">
      <PageHeader
        icon={<Blocks className="size-7" />}
        title={t("apps.title")}
        description={t("apps.description")}
        actions={
          <div className="flex flex-wrap gap-2">
            <Button size="sm" variant="outline" asChild>
              <a href={APPS_GUIDE_PATH} target="_blank" rel="noopener">
                <BookOpen className="size-4" />
                {t("apps.guide")}
              </a>
            </Button>
            {canManage && (
              <Button size="sm" onClick={() => startNew(null)}>
                <Plus className="size-4" />
                {t("apps.new")}
              </Button>
            )}
          </div>
        }
      />
      {org && !canRead && (
        <p className="text-muted-foreground text-sm">{t("apps.forbidden")}</p>
      )}
      {apps.isLoading && <Loading label={t("common.loading")} />}
      {apps.isError && (
        <ErrorState
          title={t("apps.error.title")}
          description={t("apps.error.description")}
          onRetry={() => void apps.refetch()}
        />
      )}
      {apps.isSuccess && (
        <section className="space-y-3" aria-labelledby="apps-installed">
          <h2 id="apps-installed" className="text-base font-semibold">
            {t("apps.installed.title")}
          </h2>
          {items.length === 0 ? (
            <EmptyState
              icon={<Blocks className="text-signal size-5" />}
              title={t("apps.empty")}
              description={t("apps.empty_hint")}
            />
          ) : (
            <div className="grid gap-3 md:grid-cols-2">
              {items.map((app) => (
                <InstalledAppCard
                  key={app.uuid}
                  app={app}
                  canManage={canManage}
                  toggling={toggle.isPending}
                  onToggle={() => toggle.mutate(app)}
                  onEdit={() => setEditing({ app })}
                  onDelete={() => setDeleting(app)}
                />
              ))}
            </div>
          )}
        </section>
      )}
      <section className="space-y-3" aria-labelledby="apps-gallery">
        <div>
          <h2 id="apps-gallery" className="text-base font-semibold">
            {t("apps.gallery.title")}
          </h2>
          <p className="text-muted-foreground text-sm">
            {t("apps.gallery.description")}
          </p>
        </div>
        <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
          {GALLERY.map((g) => (
            <GalleryCard
              key={g.id}
              app={g}
              canManage={canManage}
              onInstall={() => startNew(g)}
            />
          ))}
        </div>
      </section>
      <Card>
        <CardHeader>
          <CardTitle className="text-sm">
            {t("apps.developers.title")}
          </CardTitle>
          <CardDescription>{t("apps.developers.description")}</CardDescription>
        </CardHeader>
        <CardContent className="text-muted-foreground flex flex-wrap items-center gap-3 text-xs">
          <Button size="sm" variant="outline" asChild>
            <a href={APPS_GUIDE_PATH} target="_blank" rel="noopener">
              <BookOpen className="size-4" />
              {t("apps.guide")}
            </a>
          </Button>
          <span>
            {t("apps.developers.jwks")}{" "}
            <code className="bg-muted rounded px-1">
              /api/v1/app-context/jwks
            </code>
          </span>
        </CardContent>
      </Card>
      {editing && (
        <AppDialogForm
          key={
            "app" in editing
              ? editing.app.uuid
              : (editing.template?.id ?? "new")
          }
          app={"app" in editing ? editing.app : null}
          template={"app" in editing ? null : editing.template}
          initial={"app" in editing ? null : editing.initial}
          canManage={canManage}
          pending={save.isPending}
          onClose={() => setEditing(null)}
          onSubmit={(body) => save.mutate(body)}
        />
      )}
      <ConfirmDialog
        open={deleting !== null}
        title={t("apps.confirm_delete.title")}
        description={t("apps.confirm_delete.description", {
          name: deleting?.name ?? "",
        })}
        confirmLabel={t("common.delete")}
        variant="destructive"
        isPending={remove.isPending}
        onConfirm={() => deleting && remove.mutate(deleting.uuid)}
        onCancel={() => setDeleting(null)}
      />
    </div>
  );
}

/** Scope badges: what the app may read and ask for, in plain words. */
function PermissionList({ scopes }: { scopes: string[] }) {
  const { t } = useLocale();
  if (scopes.length === 0)
    return (
      <p className="text-muted-foreground text-xs">
        {t("apps.permissions.none")}
      </p>
    );
  return (
    <div className="flex flex-wrap gap-1">
      {scopes.map((s) => (
        <Badge key={s} variant="secondary">
          {t(`apps.scopes.${scopeKey(s)}`)}
        </Badge>
      ))}
    </div>
  );
}

function InstalledAppCard({
  app,
  canManage,
  toggling,
  onToggle,
  onEdit,
  onDelete,
}: {
  app: SidebarApp;
  canManage: boolean;
  toggling: boolean;
  onToggle: () => void;
  onEdit: () => void;
  onDelete: () => void;
}) {
  const { t } = useLocale();
  const brand = appBrand(app.url);
  return (
    <Card className={app.enabled ? undefined : "opacity-70"}>
      <CardContent className="flex items-start gap-3 py-4">
        <div className="bg-muted flex size-10 shrink-0 items-center justify-center rounded-lg border">
          {brand ? (
            <BrandLogo slug={brand} className="size-5" decorative />
          ) : (
            createElement(appIcon(app.icon), { className: "size-5" })
          )}
        </div>
        <div className="min-w-0 flex-1 space-y-2">
          <div className="flex flex-wrap items-center gap-2">
            <p className="truncate font-medium">{app.name}</p>
            <Badge variant="outline">
              {t(`apps.placements.${app.placement}`)}
            </Badge>
            {!app.enabled && (
              <Badge variant="secondary">{t("apps.disabled")}</Badge>
            )}
          </div>
          <p className="text-muted-foreground truncate font-mono text-xs">
            {app.url}
          </p>
          <div className="space-y-1">
            <p className="text-muted-foreground text-xs font-medium">
              {t("apps.permissions.title")}
            </p>
            <PermissionList scopes={app.scopes} />
          </div>
          <button
            type="button"
            className="text-muted-foreground hover:text-foreground inline-flex items-center gap-1 text-xs"
            onClick={() => copy(app.uuid, t("apps.toast.id_copied"))}
          >
            <Copy className="size-3" />
            {t("apps.app_id")}: <span className="font-mono">{app.uuid}</span>
          </button>
        </div>
        <div className="flex shrink-0 items-center gap-1">
          <Switch
            checked={app.enabled}
            disabled={!canManage || toggling}
            aria-label={t("apps.fields.enabled")}
            onCheckedChange={onToggle}
          />
          <Button
            size="icon"
            variant="ghost"
            aria-label={canManage ? t("apps.edit") : t("apps.view")}
            onClick={onEdit}
          >
            <Pencil className="size-4" />
          </Button>
          {canManage && (
            <Button
              size="icon"
              variant="ghost"
              aria-label={t("apps.delete")}
              onClick={onDelete}
            >
              <Trash2 className="size-4" />
            </Button>
          )}
        </div>
      </CardContent>
    </Card>
  );
}

function GalleryCard({
  app,
  canManage,
  onInstall,
}: {
  app: GalleryApp;
  canManage: boolean;
  onInstall: () => void;
}) {
  const { t } = useLocale();
  const brand = galleryBrand(app);
  return (
    <Card className="flex flex-col">
      <CardContent className="flex flex-1 flex-col gap-3 py-4">
        <div className="flex items-start gap-3">
          <div className="bg-muted flex size-10 shrink-0 items-center justify-center rounded-lg border">
            {brand ? (
              <BrandLogo slug={brand} className="size-5" decorative />
            ) : (
              createElement(appIcon(app.icon), { className: "size-5" })
            )}
          </div>
          <div className="min-w-0 space-y-1">
            <p className="font-medium">{t(`apps.gallery.${app.id}.name`)}</p>
            <Badge variant="outline">
              {t(`apps.placements.${app.placement}`)}
            </Badge>
          </div>
        </div>
        <p className="text-muted-foreground flex-1 text-sm">
          {t(`apps.gallery.${app.id}.description`)}
        </p>
        <div className="space-y-1">
          <p className="text-muted-foreground text-xs font-medium">
            {t("apps.permissions.title")}
          </p>
          <PermissionList scopes={app.scopes} />
        </div>
        <div className="flex flex-wrap items-center gap-2">
          {canManage && (
            <Button size="sm" onClick={onInstall}>
              <Plus className="size-4" />
              {t("apps.gallery.install")}
            </Button>
          )}
          <Button size="sm" variant="ghost" asChild>
            <a href={app.docsUrl} target="_blank" rel="noopener noreferrer">
              {t("apps.gallery.learn_more")}
            </a>
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}

function AppDialogForm({
  app,
  template,
  initial,
  canManage,
  pending,
  onClose,
  onSubmit,
}: {
  app: SidebarApp | null;
  template: GalleryApp | null;
  initial: SidebarAppInput | null;
  canManage: boolean;
  pending: boolean;
  onClose: () => void;
  onSubmit: (body: SidebarAppInput) => void;
}) {
  const { t } = useLocale();
  const [form, setForm] = useState<SidebarAppInput>(() =>
    app
      ? {
          name: app.name,
          url: app.url,
          icon: app.icon || "puzzle",
          placement: app.placement,
          scopes: app.scopes,
          enabled: app.enabled,
          revision: app.revision,
        }
      : (initial as SidebarAppInput),
  );
  const [preview, setPreview] = useState(false);
  const disabled = !canManage || pending;
  const required = REQUIRED_SCOPE[form.placement];
  const scopes =
    required && !form.scopes.includes(required)
      ? [required, ...form.scopes]
      : form.scopes;
  const placeholder = hasPlaceholder(form.url);
  const urlInvalid = placeholder || !validAppUrl(form.url);
  const canPreview =
    !urlInvalid && new URL(form.url).origin !== window.location.origin;
  const nameInvalid = !form.name.trim() || form.name.length > 120;
  // Write scopes only work with conversation context.
  const actionScopesUseless =
    form.placement !== "conversation_sidebar" &&
    scopes.some((s) => s !== "conversation.read" && s !== "contact.read");

  return (
    <AppDialog
      open
      onOpenChange={(o) => !o && onClose()}
      title={
        app
          ? t("apps.dialog.edit")
          : template
            ? t("apps.dialog.install", {
                name: t(`apps.gallery.${template.id}.name`),
              })
            : t("apps.dialog.create")
      }
      description={
        template
          ? t(`apps.gallery.${template.id}.setup`)
          : t("apps.dialog.description")
      }
      className="max-h-[90vh] overflow-y-auto sm:max-w-xl"
    >
      <form
        className="space-y-4"
        onSubmit={(e) => {
          e.preventDefault();
          if (!urlInvalid && !nameInvalid && canManage)
            onSubmit({ ...form, name: form.name.trim(), scopes });
        }}
      >
        <div className="space-y-2">
          <Label htmlFor="app-name">{t("apps.fields.name")}</Label>
          <Input
            id="app-name"
            value={form.name}
            maxLength={120}
            disabled={disabled}
            aria-invalid={nameInvalid}
            onChange={(e) => setForm({ ...form, name: e.target.value })}
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor="app-url">{t("apps.fields.url")}</Label>
          <Input
            id="app-url"
            value={form.url}
            maxLength={2048}
            inputMode="url"
            disabled={disabled}
            aria-invalid={urlInvalid}
            className="font-mono text-xs"
            onChange={(e) => setForm({ ...form, url: e.target.value.trim() })}
          />
          <p
            className={
              urlInvalid
                ? "text-destructive text-xs"
                : "text-muted-foreground text-xs"
            }
          >
            {placeholder
              ? t("apps.fields.url_placeholder")
              : t("apps.fields.url_help")}
          </p>
          {canPreview && (
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() => setPreview((v) => !v)}
            >
              <Eye className="size-4" />
              {preview ? t("apps.preview.hide") : t("apps.preview.show")}
            </Button>
          )}
          {canPreview && preview && (
            <div className="space-y-1">
              <iframe
                src={form.url}
                title={t("apps.preview.title", { name: form.name })}
                sandbox="allow-scripts allow-forms allow-same-origin"
                referrerPolicy="no-referrer"
                allow=""
                className="bg-background h-80 w-full rounded-md border"
              />
              <p className="text-muted-foreground text-xs">
                {t("apps.preview.hint")}
              </p>
            </div>
          )}
        </div>
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-2">
            <Label htmlFor="app-icon">{t("apps.fields.icon")}</Label>
            <Select
              value={form.icon}
              disabled={disabled}
              onValueChange={(icon) => setForm({ ...form, icon })}
            >
              <SelectTrigger id="app-icon" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {Object.entries(APP_ICONS).map(([id, Icon]) => (
                  <SelectItem key={id} value={id}>
                    <Icon className="size-4" />
                    {id}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-2">
            <Label htmlFor="app-placement">{t("apps.fields.placement")}</Label>
            <Select
              value={form.placement}
              disabled={disabled}
              onValueChange={(p) =>
                setForm({ ...form, placement: p as SidebarAppPlacement })
              }
            >
              <SelectTrigger id="app-placement" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {PLACEMENTS.map((p) => (
                  <SelectItem key={p} value={p}>
                    {t(`apps.placements.${p}`)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </div>
        <fieldset className="space-y-2">
          <legend className="text-sm font-medium">
            {t("apps.fields.scopes")}
          </legend>
          <p className="text-muted-foreground text-xs">
            {t("apps.fields.scopes_help")}
          </p>
          <ul className="space-y-1.5">
            {SCOPES.map((s) => {
              const locked = s === required;
              const checked = scopes.includes(s);
              return (
                <li key={s}>
                  <label className="flex items-start gap-2 text-sm">
                    <Checkbox
                      checked={checked}
                      disabled={disabled || locked}
                      onCheckedChange={(v) =>
                        setForm({
                          ...form,
                          scopes: v
                            ? [...form.scopes.filter((x) => x !== s), s]
                            : form.scopes.filter((x) => x !== s),
                        })
                      }
                    />
                    <span className="min-w-0">
                      <span className="block">
                        {t(`apps.scopes.${scopeKey(s)}`)}
                      </span>
                      <span className="text-muted-foreground block text-xs">
                        {t("apps.fields.requires", {
                          permission: t(
                            `permissions.labels.${SCOPE_PERMISSION[s]}`,
                          ),
                        })}
                      </span>
                    </span>
                  </label>
                </li>
              );
            })}
          </ul>
          {actionScopesUseless && (
            <p className="text-xs text-amber-700 dark:text-amber-300">
              {t("apps.fields.actions_need_conversation")}
            </p>
          )}
        </fieldset>
        <label className="flex items-center justify-between gap-3 text-sm">
          {t("apps.fields.enabled")}
          <Switch
            checked={form.enabled}
            disabled={disabled}
            onCheckedChange={(enabled) => setForm({ ...form, enabled })}
          />
        </label>
        <FormActions sticky={false}>
          <Button type="button" variant="outline" onClick={onClose}>
            {canManage ? t("common.cancel") : t("common.close")}
          </Button>
          {canManage && (
            <Button
              type="submit"
              disabled={pending || urlInvalid || nameInvalid}
            >
              {t("common.save")}
            </Button>
          )}
        </FormActions>
      </form>
    </AppDialog>
  );
}
