"use client";

import { Pencil, Plus, Trash2 } from "lucide-react";
import { useState } from "react";

import { Can } from "@/components/common/can";
import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { EntityDeleteDialog, EntityPage } from "@/components/entity";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { StatusDot } from "@/features/conversation-statuses/components/status-dot";
import { StatusFormDialog } from "@/features/conversation-statuses/components/status-form-dialog";
import {
  useConversationStatuses,
  useDeleteConversationStatus,
} from "@/features/conversation-statuses/hooks/use-conversation-statuses";
import type { CustomStatus } from "@/features/conversation-statuses/types";
import { usePermission } from "@/providers/permission-provider";
import { useLocale } from "@/providers/locale-provider";

type FormState = { status?: CustomStatus } | null;

/**
 * Custom conversation statuses (CF-249): the organization's own names on
 * top of the base statuses (open, pending, resolved), e.g. "İşleniyor".
 * Agents pick them in the conversation header.
 */
export function ConversationStatusesPage() {
  const { t } = useLocale();
  const { can } = usePermission();
  const [form, setForm] = useState<FormState>(null);
  const [deleteTarget, setDeleteTarget] = useState<CustomStatus | null>(null);
  const list = useConversationStatuses();
  const remove = useDeleteConversationStatus();
  const canManage = can(permissions.tenant.conversations.manage);
  const items = list.data?.items ?? [];

  return (
    <EntityPage
      title={t("conversation_statuses.title")}
      description={t("conversation_statuses.description")}
      permission={permissions.tenant.conversations.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("conversation_statuses.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.cms.home },
        { label: t("conversation_statuses.title") },
      ]}
      actions={
        <Can permission={permissions.tenant.conversations.manage}>
          <Button onClick={() => setForm({})}>
            <Plus />
            {t("conversation_statuses.actions.create")}
          </Button>
        </Can>
      }
    >
      {list.isError ? (
        <ErrorState
          title={t("conversation_statuses.load_failed")}
          onRetry={() => void list.refetch()}
        />
      ) : !list.isLoading && items.length === 0 ? (
        <EmptyState
          title={t("conversation_statuses.empty_title")}
          description={t("conversation_statuses.empty_description")}
        />
      ) : (
        <ul className="divide-y rounded-lg border">
          {items.map((s) => (
            <li key={s.uuid} className="flex items-center gap-3 px-3 py-2.5">
              <StatusDot color={s.color} className="size-2.5" />
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="truncate font-medium">{s.name}</span>
                  <Badge variant="outline">
                    {t(`inbox.status.${s.category}`)}
                  </Badge>
                </div>
                <p className="text-muted-foreground text-xs">
                  {t("conversation_statuses.usage", { count: s.usage_count })}
                </p>
              </div>
              {canManage ? (
                <div className="flex shrink-0 items-center gap-1">
                  <Button
                    size="icon-sm"
                    variant="ghost"
                    aria-label={t("conversation_statuses.actions.edit")}
                    onClick={() => setForm({ status: s })}
                  >
                    <Pencil />
                  </Button>
                  <Button
                    size="icon-sm"
                    variant="ghost"
                    aria-label={t("conversation_statuses.actions.delete")}
                    onClick={() => setDeleteTarget(s)}
                  >
                    <Trash2 />
                  </Button>
                </div>
              ) : null}
            </li>
          ))}
        </ul>
      )}

      <StatusFormDialog
        open={form !== null}
        onOpenChange={(open) => !open && setForm(null)}
        status={form?.status}
      />
      <EntityDeleteDialog
        open={Boolean(deleteTarget)}
        softDelete={false}
        entityLabel={deleteTarget?.name ?? ""}
        isPending={remove.isPending}
        onCancel={() => setDeleteTarget(null)}
        onConfirm={() => {
          if (!deleteTarget) return;
          remove.mutate(deleteTarget.uuid, {
            onSettled: () => setDeleteTarget(null),
          });
        }}
      />
    </EntityPage>
  );
}
