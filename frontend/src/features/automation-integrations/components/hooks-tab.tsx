"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Trash2, Webhook } from "lucide-react";
import { useState } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { StatusChip } from "@/components/common/status-chip";
import { DeleteDialog } from "@/components/dialogs/delete-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { relativeDatetime } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

import {
  automationIntegrationsService,
  type HookSubscription,
} from "../services/automation-integrations.service";
import { useIntegrationsError } from "./api-keys-tab";

/**
 * REST hook subscriptions of the organization. Zapier, Make and n8n create
 * them through `POST /subscriptions`; removing one here stops deliveries.
 */
export function HooksTab({
  slug,
  canDelete,
}: {
  slug: string;
  canDelete: boolean;
}) {
  const { t, locale } = useLocale();
  const qc = useQueryClient();
  const onError = useIntegrationsError();
  const key = ["tenant", slug, "webhooks"] as const;
  const hooks = useQuery({
    queryKey: key,
    queryFn: () => automationIntegrationsService.hooks(slug),
  });
  const [deleting, setDeleting] = useState<HookSubscription | null>(null);
  const remove = useMutation({
    mutationFn: (uuid: string) =>
      automationIntegrationsService.removeHook(slug, uuid),
    onSuccess: async () => {
      setDeleting(null);
      await qc.invalidateQueries({ queryKey: key });
      appToast.success(t("automation_integrations.hooks.deleted_toast"));
    },
    onError,
  });

  if (hooks.isLoading) return <Loading label={t("common.loading")} />;
  if (hooks.isError)
    return (
      <ErrorState
        title={t("automation_integrations.error.title")}
        description={t("automation_integrations.error.description")}
        onRetry={() => void hooks.refetch()}
      />
    );
  const items = hooks.data ?? [];

  return (
    <div className="space-y-4">
      <p className="text-muted-foreground text-sm">
        {t("automation_integrations.hooks.hint")}
      </p>
      {items.length === 0 ? (
        <EmptyState
          icon={<Webhook className="size-6" />}
          title={t("automation_integrations.hooks.empty_title")}
          description={t("automation_integrations.hooks.empty_description")}
        />
      ) : (
        <ul className="divide-y rounded-md border">
          {items.map((h) => (
            <li key={h.uuid} className="flex flex-wrap items-start gap-3 p-3">
              <div className="min-w-0 flex-1 space-y-1">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="font-medium">{h.name}</span>
                  {h.enabled ? (
                    <StatusChip
                      label={t("automation_integrations.hooks.enabled")}
                      tone="success"
                    />
                  ) : (
                    <StatusChip
                      label={t("automation_integrations.hooks.disabled")}
                    />
                  )}
                </div>
                <p className="text-muted-foreground truncate font-mono text-xs">
                  {h.url}
                </p>
                <div className="flex flex-wrap gap-1">
                  {h.events.map((e) => (
                    <Badge key={e} variant="outline" className="font-mono">
                      {e}
                    </Badge>
                  ))}
                </div>
                <p className="text-muted-foreground text-xs">
                  {t("automation_integrations.hooks.created", {
                    time: relativeDatetime(h.created_at, locale),
                  })}
                </p>
              </div>
              {canDelete && (
                <Button
                  size="icon"
                  variant="ghost"
                  aria-label={t("automation_integrations.hooks.delete")}
                  onClick={() => setDeleting(h)}
                >
                  <Trash2 className="size-4" />
                </Button>
              )}
            </li>
          ))}
        </ul>
      )}
      <DeleteDialog
        open={Boolean(deleting)}
        title={t("automation_integrations.hooks.delete_title", {
          name: deleting?.name ?? "",
        })}
        description={t("automation_integrations.hooks.delete_description")}
        isPending={remove.isPending}
        onConfirm={() => deleting && remove.mutate(deleting.uuid)}
        onCancel={() => setDeleting(null)}
      />
    </div>
  );
}
