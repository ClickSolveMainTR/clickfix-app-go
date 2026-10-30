"use client";

import { Building2, ImageUp, Trash2 } from "lucide-react";
import { useRef, useState } from "react";

import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import { EntitySectionCard } from "@/components/entity";
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { permissions } from "@/config/permissions";
import {
  useRemoveOrganizationLogo,
  useUploadOrganizationLogo,
} from "@/features/tenant-settings/hooks/use-tenant-settings";
import type { OrganizationProfile } from "@/features/tenant-settings/types";
import { resolveLogoSrc } from "@/lib/branding/logo-url";
import { assertServiceMediaURL } from "@/lib/media/urls";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

const ACCEPT = "image/png,image/jpeg,image/webp,image/svg+xml";
const MAX_BYTES = 2 * 1024 * 1024;

/**
 * Organization logo: uploaded through the API and shown from the API-owned
 * `logo_url` only (never an object key).
 */
export function OrganizationLogoCard({
  profile,
}: {
  profile: OrganizationProfile | undefined;
}) {
  const { t } = useLocale();
  const { can } = usePermission();
  const canWrite = can(permissions.tenant.settings.write);
  const fileInput = useRef<HTMLInputElement>(null);
  const [confirmRemove, setConfirmRemove] = useState(false);
  const upload = useUploadOrganizationLogo();
  const removeLogo = useRemoveOrganizationLogo();

  const logoSrc = resolveLogoSrc(assertServiceMediaURL(profile?.logo_url));

  return (
    <EntitySectionCard title={t("tenant_settings.profile.title")}>
      {!profile ? (
        <Skeleton className="h-20 w-full" />
      ) : (
        <div className="flex flex-col gap-6 md:flex-row md:items-start">
          <div className="flex items-center gap-4">
            <Avatar className="size-20 rounded-xl border">
              {logoSrc ? (
                <AvatarImage
                  src={logoSrc}
                  alt=""
                  className="object-contain p-1"
                />
              ) : null}
              <AvatarFallback className="rounded-xl">
                <Building2 className="text-muted-foreground size-8" />
              </AvatarFallback>
            </Avatar>
            {canWrite ? (
              <div className="flex flex-col gap-2">
                <Input
                  ref={fileInput}
                  type="file"
                  accept={ACCEPT}
                  className="hidden"
                  aria-label={t("tenant_settings.profile.logo_upload")}
                  onChange={(event) => {
                    const file = event.target.files?.[0];
                    event.target.value = "";
                    if (!file) return;
                    if (file.size > MAX_BYTES) {
                      appToast.error(
                        t("tenant_settings.profile.logo_too_large"),
                      );
                      return;
                    }
                    upload.mutate(file);
                  }}
                />
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  disabled={upload.isPending}
                  onClick={() => fileInput.current?.click()}
                >
                  <ImageUp className="size-4" />
                  {profile.logo_url
                    ? t("tenant_settings.profile.logo_replace")
                    : t("tenant_settings.profile.logo_upload")}
                </Button>
                {profile.logo_url ? (
                  <Button
                    type="button"
                    size="sm"
                    variant="ghost"
                    className="text-destructive"
                    onClick={() => setConfirmRemove(true)}
                  >
                    <Trash2 className="size-4" />
                    {t("tenant_settings.profile.logo_remove")}
                  </Button>
                ) : null}
                <p className="text-muted-foreground text-xs">
                  {t("tenant_settings.profile.logo_hint")}
                </p>
              </div>
            ) : null}
          </div>
        </div>
      )}
      <ConfirmDialog
        open={confirmRemove}
        title={t("tenant_settings.profile.logo_remove_title")}
        description={t("tenant_settings.profile.logo_remove_description")}
        confirmLabel={t("tenant_settings.profile.logo_remove")}
        variant="destructive"
        isPending={removeLogo.isPending}
        onCancel={() => setConfirmRemove(false)}
        onConfirm={() =>
          removeLogo.mutate(undefined, {
            onSuccess: () => setConfirmRemove(false),
          })
        }
      />
    </EntitySectionCard>
  );
}
