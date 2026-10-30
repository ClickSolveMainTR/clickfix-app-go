"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Globe, Mail, Palette, Pencil, Plus, Trash2 } from "lucide-react";
import { useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import { PageHeader } from "@/components/layout/page-header";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Permission } from "@/config/permissions";
import { useActiveOrganization } from "@/features/organizations";
import { isApiError } from "@/lib/api";
import { resolveLogoSrc } from "@/lib/branding/logo-url";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

import { brandKeys, useBrands } from "../hooks/use-brands";
import {
  brandsService,
  type Brand,
  type BrandInput,
} from "../services/brands.service";
import { BrandDialog } from "./brand-dialog";

/**
 * Brands of the organization (CF-124). Each brand carries its own identity,
 * e-mail sender, help center address, widget defaults, hours and SLA. Brand
 * count is limited by the plan (`brands` entitlement). Reuses the channel
 * permissions like the backend.
 */
export function BrandsPage() {
  const { t } = useLocale();
  const qc = useQueryClient();
  const { active: org } = useActiveOrganization();
  const perms = org?.permissions ?? [];
  const canRead = perms.includes(Permission.TenantChannelsRead);
  const canManage = perms.includes(Permission.TenantChannelsManage);
  const brands = useBrands();
  const [editing, setEditing] = useState<Brand | "new" | null>(null);
  const [deleting, setDeleting] = useState<Brand | null>(null);
  const key = brandKeys.all(org?.slug);

  const onError = (error: unknown) =>
    appToast.error(
      isApiError(error) ? error.message : t("brands.toast.failed"),
    );
  const save = useMutation({
    mutationFn: (body: BrandInput) =>
      editing && editing !== "new"
        ? brandsService.replace(editing.uuid, body)
        : brandsService.create(body),
    onSuccess: async () => {
      setEditing(null);
      await qc.invalidateQueries({ queryKey: key });
      appToast.success(t("brands.toast.saved"));
    },
    onError,
  });
  const remove = useMutation({
    mutationFn: (uuid: string) => brandsService.remove(uuid),
    onSuccess: async () => {
      setDeleting(null);
      await qc.invalidateQueries({ queryKey: key });
      appToast.success(t("brands.toast.deleted"));
    },
    onError: (error) => {
      setDeleting(null);
      appToast.error(
        isApiError(error) && error.status === 409
          ? t("brands.toast.in_use")
          : isApiError(error)
            ? error.message
            : t("brands.toast.failed"),
      );
    },
  });

  const items = brands.data ?? [];

  return (
    <div className="space-y-6">
      <PageHeader
        icon={<Palette className="size-7" />}
        title={t("brands.title")}
        description={t("brands.description")}
        actions={
          canManage ? (
            <Button size="sm" onClick={() => setEditing("new")}>
              <Plus className="size-4" />
              {t("brands.new")}
            </Button>
          ) : undefined
        }
      />
      {org && !canRead && (
        <p className="text-muted-foreground text-sm">{t("brands.forbidden")}</p>
      )}
      {brands.isLoading && <Loading label={t("common.loading")} />}
      {brands.isError && (
        <ErrorState
          title={t("brands.error.title")}
          description={t("brands.error.description")}
          onRetry={() => void brands.refetch()}
        />
      )}
      {brands.isSuccess && items.length === 0 && (
        <Card>
          <CardContent className="text-muted-foreground py-8 text-center text-sm">
            {t("brands.empty")}
          </CardContent>
        </Card>
      )}
      <div className="grid gap-3 md:grid-cols-2">
        {items.map((b) => {
          const logo = resolveLogoSrc(b.logo_url);
          return (
            <Card key={b.uuid} className="overflow-hidden">
              <div
                aria-hidden
                className="h-1.5"
                style={{ backgroundColor: b.primary_color ?? "#2563eb" }}
              />
              <CardContent className="flex items-start gap-3 py-4">
                <div className="bg-muted flex size-11 shrink-0 items-center justify-center overflow-hidden rounded-lg border">
                  {logo ? (
                    // eslint-disable-next-line @next/next/no-img-element
                    <img
                      src={logo}
                      alt=""
                      className="size-full object-contain"
                    />
                  ) : (
                    <span
                      className="text-sm font-semibold"
                      style={{ color: b.primary_color ?? undefined }}
                    >
                      {b.name.slice(0, 2).toUpperCase()}
                    </span>
                  )}
                </div>
                <div className="min-w-0 flex-1 space-y-1.5">
                  <p className="truncate font-medium">{b.name}</p>
                  <div className="text-muted-foreground flex flex-col gap-1 text-xs">
                    {b.support_email && (
                      <span className="flex items-center gap-1.5 truncate">
                        <Mail className="size-3.5 shrink-0" />
                        <span className="truncate">
                          {b.sender_name
                            ? `${b.sender_name} <${b.support_email}>`
                            : b.support_email}
                        </span>
                      </span>
                    )}
                    {(b.help_subdomain || b.help_path) && (
                      <span className="flex items-center gap-1.5 truncate">
                        <Globe className="size-3.5 shrink-0" />
                        <span className="truncate">
                          {b.help_subdomain || `/${b.help_path}`}
                        </span>
                      </span>
                    )}
                  </div>
                  <div className="flex flex-wrap gap-1">
                    {b.business_hours?.enabled && (
                      <Badge variant="outline">{t("brands.badge.hours")}</Badge>
                    )}
                    {b.sla_policy_id && (
                      <Badge variant="outline">{t("brands.badge.sla")}</Badge>
                    )}
                    {b.signature && (
                      <Badge variant="outline">
                        {t("brands.badge.signature")}
                      </Badge>
                    )}
                  </div>
                </div>
                <div className="flex shrink-0 gap-1">
                  <Button
                    size="icon"
                    variant="ghost"
                    aria-label={canManage ? t("brands.edit") : t("brands.view")}
                    onClick={() => setEditing(b)}
                  >
                    <Pencil className="size-4" />
                  </Button>
                  {canManage && (
                    <Button
                      size="icon"
                      variant="ghost"
                      aria-label={t("brands.delete")}
                      onClick={() => setDeleting(b)}
                    >
                      <Trash2 className="size-4" />
                    </Button>
                  )}
                </div>
              </CardContent>
            </Card>
          );
        })}
      </div>
      {editing && (
        <BrandDialog
          open
          onOpenChange={(o) => !o && setEditing(null)}
          brand={editing === "new" ? null : editing}
          canManage={canManage}
          pending={save.isPending}
          onSubmit={(body) => save.mutate(body)}
        />
      )}
      <ConfirmDialog
        open={deleting !== null}
        title={t("brands.confirm_delete.title")}
        description={t("brands.confirm_delete.description", {
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
