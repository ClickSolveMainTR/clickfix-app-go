"use client";

import { useMutation } from "@tanstack/react-query";
import { LogOut } from "lucide-react";
import { useRouter } from "next/navigation";
import { useState } from "react";

import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import { EntitySectionCard } from "@/components/entity";
import { Button } from "@/components/ui/button";
import { routes } from "@/config/routes";
import { useActiveOrganizationStore } from "@/features/organizations";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantService } from "@/features/tenant/services/tenant.service";
import { isApiError } from "@/lib/api";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/**
 * "Leave organization" for any member (`POST /v1/tenant/leave`). The last
 * owner gets `409 LAST_ORGANIZATION_OWNER`.
 */
export function LeaveOrganizationCard() {
  const { t } = useLocale();
  const { slug, organization } = useTenant();
  const { hydrateProfile } = useAuth();
  const setActiveSlug = useActiveOrganizationStore((s) => s.setSlug);
  const router = useRouter();
  const [confirm, setConfirm] = useState(false);
  const name = organization?.name ?? slug;

  const leave = useMutation({
    mutationFn: () => tenantService.leave(slug),
    onSuccess: async () => {
      appToast.success(t("tenant.home.leave_done"));
      setActiveSlug(null);
      await hydrateProfile();
      router.replace(routes.cms.home);
    },
    onError: (error) => {
      appToast.error(
        isApiError(error) && error.code === "LAST_ORGANIZATION_OWNER"
          ? t("tenant.home.leave_last_owner")
          : t("common.error_generic"),
      );
    },
    onSettled: () => setConfirm(false),
  });

  return (
    <EntitySectionCard title={t("tenant.home.leave")}>
      <div className="flex flex-col items-start gap-3">
        <p className="text-muted-foreground text-sm">
          {t("tenant.home.leave_body", { name })}
        </p>
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="text-destructive"
          onClick={() => setConfirm(true)}
        >
          <LogOut className="size-4" />
          {t("tenant.home.leave")}
        </Button>
      </div>
      <ConfirmDialog
        open={confirm}
        title={t("tenant.home.leave_title")}
        description={t("tenant.home.leave_body", { name })}
        confirmLabel={t("tenant.home.leave_confirm")}
        variant="destructive"
        isPending={leave.isPending}
        onCancel={() => setConfirm(false)}
        onConfirm={() => leave.mutate()}
      />
    </EntitySectionCard>
  );
}
