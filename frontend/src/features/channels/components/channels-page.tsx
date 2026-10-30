"use client";

import { Plus } from "lucide-react";
import { useState } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import { EntityPage } from "@/components/entity";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { permissions } from "@/config/permissions";
import { AddChannelDialog } from "@/features/channels/components/add-channel-dialog";
import {
  ChannelCard,
  type ChannelCardActions,
} from "@/features/channels/components/channel-card";
import { ChannelSettingsDialog } from "@/features/channels/components/channel-settings-dialog";
import { WhatsAppTemplatesDialog } from "@/features/channels/components/whatsapp-templates-dialog";
import {
  useChannels,
  useCheckChannel,
  useDeleteChannel,
  useReconnectChannel,
  useUpdateChannel,
} from "@/features/channels/hooks/use-channels";
import type { ChannelInstance } from "@/features/channels/types";
import { usePermission } from "@/providers/permission-provider";
import { useLocale } from "@/providers/locale-provider";

/**
 * Channel management (CF-61): every inbox of the organization with its
 * health, the add wizard, per-inbox settings, WhatsApp templates, pause,
 * reconnect and disconnect. The number of inboxes is limited by the plan
 * (the API answers PLAN_LIMIT_EXCEEDED).
 */
export function ChannelsPage() {
  const { t } = useLocale();
  const { can } = usePermission();
  const canManage = can(permissions.tenant.channels.manage);
  const channels = useChannels();
  const check = useCheckChannel();
  const reconnect = useReconnectChannel();
  const update = useUpdateChannel();
  const remove = useDeleteChannel();
  const [adding, setAdding] = useState(false);
  const [settings, setSettings] = useState<ChannelInstance | null>(null);
  const [templates, setTemplates] = useState<ChannelInstance | null>(null);
  const [deleting, setDeleting] = useState<ChannelInstance | null>(null);

  const actions: ChannelCardActions = {
    onCheck: (c) => check.mutate(c.uuid),
    onReconnect: (c) => reconnect.mutate(c.uuid),
    onToggle: (c) =>
      update.mutate({
        uuid: c.uuid,
        body: { status: c.status === "paused" ? "active" : "paused" },
      }),
    onSettings: setSettings,
    onTemplates: setTemplates,
    onDelete: setDeleting,
  };
  const busy = check.isPending || reconnect.isPending || update.isPending;
  const items = channels.data ?? [];

  return (
    <EntityPage
      title={t("channels.title")}
      description={t("channels.description")}
      permission={permissions.tenant.channels.read}
      actions={
        canManage ? (
          <Button onClick={() => setAdding(true)}>
            <Plus className="size-4" />
            {t("channels.add.button")}
          </Button>
        ) : null
      }
    >
      {channels.isError ? (
        <ErrorState
          title={t("channels.error")}
          onRetry={() => void channels.refetch()}
        />
      ) : channels.isLoading ? (
        <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
          {[0, 1, 2].map((i) => (
            <Skeleton key={i} className="h-64" />
          ))}
        </div>
      ) : items.length === 0 ? (
        <EmptyState
          title={t("channels.empty.title")}
          description={t("channels.empty.description")}
        />
      ) : (
        <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
          {items.map((c) => (
            <ChannelCard
              key={c.uuid}
              channel={c}
              canManage={canManage}
              busy={busy}
              actions={actions}
            />
          ))}
        </div>
      )}

      <AddChannelDialog open={adding} onOpenChange={setAdding} />
      {settings ? (
        <ChannelSettingsDialog
          channel={settings}
          onOpenChange={(open) => !open && setSettings(null)}
        />
      ) : null}
      {templates ? (
        <WhatsAppTemplatesDialog
          channel={templates}
          onOpenChange={(open) => !open && setTemplates(null)}
        />
      ) : null}
      <ConfirmDialog
        open={deleting !== null}
        variant="destructive"
        title={t("channels.delete.title", { name: deleting?.name ?? "" })}
        description={t("channels.delete.description")}
        confirmLabel={t("channels.actions.disconnect")}
        isPending={remove.isPending}
        onCancel={() => setDeleting(null)}
        onConfirm={() =>
          deleting &&
          remove.mutate(deleting.uuid, { onSuccess: () => setDeleting(null) })
        }
      />
    </EntityPage>
  );
}
