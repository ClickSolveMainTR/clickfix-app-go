"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Copy, MessageCircle, Plus, Trash2 } from "lucide-react";
import { useRef, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { PageHeader } from "@/components/layout/page-header";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Permission } from "@/config/permissions";
import { useActiveOrganization } from "@/features/organizations";
import {
  WidgetEditor,
  type InstancePatch,
} from "@/features/widget/components/widget-editor";
import { DEFAULT_SETTINGS } from "@/features/widget/lib/settings";
import {
  widgetConfigService,
  widgetService,
} from "@/features/widget/services/widget.service";
import { isApiError } from "@/lib/api";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";
import { useDialogs } from "@/providers/dialog-provider";

/**
 * Web widget settings (CF-55, CF-150): an organization's widgets (channel
 * instances of type `widget`, one per site or brand) and the editor with
 * live preview, autosaved drafts, publishing and version history. Reading
 * needs tenant.channels.read, editing tenant.channels.manage, publishing
 * tenant.widgets.publish.
 */
export function WidgetAdminPage() {
  const { confirmDelete } = useDialogs();
  const { t } = useLocale();
  const queryClient = useQueryClient();
  const { active: org } = useActiveOrganization();
  const perms = org?.permissions ?? [];
  const canRead = perms.includes(Permission.TenantChannelsRead);
  const canManage = perms.includes(Permission.TenantChannelsManage);
  const canPublish =
    canManage && perms.includes(Permission.TenantWidgetsPublish);
  const [selected, setSelected] = useState<string | null>(null);
  const [newName, setNewName] = useState("");
  const flushRef = useRef<(() => Promise<void>) | null>(null);
  const queryKey = ["tenant", org?.slug, "widgets"] as const;

  const { data, isLoading, isError, refetch } = useQuery({
    queryKey,
    queryFn: () => widgetService.list(),
    enabled: Boolean(org) && canRead,
  });

  const onError = (error: unknown) => {
    appToast.error(
      isApiError(error) ? error.message : t("widget.toast.failed"),
    );
  };

  const select = async (uuid: string) => {
    // Never lose an edit waiting for its autosave.
    await flushRef.current?.();
    setSelected(uuid);
  };

  const create = useMutation({
    mutationFn: (name: string) =>
      widgetService.create({
        name,
        settings: DEFAULT_SETTINGS as unknown as Record<string, unknown>,
      }),
    onSuccess: async (w) => {
      setNewName("");
      await queryClient.invalidateQueries({ queryKey });
      await select(w.uuid);
      appToast.success(t("widget.toast.created"));
    },
    onError,
  });

  const duplicate = useMutation({
    mutationFn: (uuid: string) => widgetConfigService.duplicate(uuid, ""),
    onSuccess: async (w) => {
      await queryClient.invalidateQueries({ queryKey });
      await select(w.uuid);
      appToast.success(t("widget.toast.duplicated"));
    },
    onError,
  });

  const remove = useMutation({
    mutationFn: (uuid: string) => widgetService.remove(uuid),
    onSuccess: async () => {
      setSelected(null);
      await queryClient.invalidateQueries({ queryKey });
      appToast.success(t("widget.toast.deleted"));
    },
    onError,
  });

  const widgets = data ?? [];
  const current =
    widgets.find((w) => w.uuid === selected) ?? widgets[0] ?? null;

  const saveInstance = async (patch: InstancePatch) => {
    if (!current) return;
    try {
      await widgetService.update(current.uuid, {
        ...(patch.name !== undefined ? { name: patch.name } : {}),
        ...(patch.status !== undefined ? { status: patch.status } : {}),
        ...(patch.brandId !== undefined ? { brand_id: patch.brandId } : {}),
        ...(patch.identitySecret
          ? { credentials: { identity_secret: patch.identitySecret } }
          : {}),
      });
      await queryClient.invalidateQueries({ queryKey });
      appToast.success(t("widget.toast.saved"));
    } catch (err) {
      onError(err);
    }
  };

  return (
    <div className="space-y-6">
      <PageHeader
        icon={<MessageCircle className="size-7" />}
        title={t("widget.title")}
        description={t("widget.description")}
      />

      {!org ? (
        <p className="text-muted-foreground text-sm">
          {t("widget.no_organization")}
        </p>
      ) : null}
      {org && !canRead ? (
        <p className="text-muted-foreground text-sm">{t("widget.forbidden")}</p>
      ) : null}
      {isLoading ? <Loading label={t("common.loading")} /> : null}
      {isError ? (
        <ErrorState
          title={t("widget.error.title")}
          description={t("widget.error.description")}
          retryLabel={t("common.retry")}
          onRetry={() => refetch()}
        />
      ) : null}

      {data ? (
        <div className="space-y-4">
          <div
            className="flex flex-wrap items-center gap-2"
            role="tablist"
            aria-label={t("widget.list")}
          >
            {widgets.map((w) => (
              <div
                key={w.uuid}
                className={cn(
                  "flex items-center rounded-lg border",
                  current?.uuid === w.uuid && "border-primary bg-accent",
                )}
              >
                <button
                  type="button"
                  role="tab"
                  aria-selected={current?.uuid === w.uuid}
                  onClick={() => void select(w.uuid)}
                  className="flex items-center gap-2 px-3 py-1.5 text-sm"
                >
                  <span
                    className="size-3 shrink-0 rounded-full"
                    style={{
                      background: String(
                        w.settings?.color ?? DEFAULT_SETTINGS.color,
                      ),
                    }}
                  />
                  <span className="max-w-40 truncate">{w.name}</span>
                  {w.status === "paused" ? (
                    <span className="text-muted-foreground text-xs">
                      ({t("channels.status.paused")})
                    </span>
                  ) : null}
                </button>
                {canManage && current?.uuid === w.uuid ? (
                  <>
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon"
                      className="size-7"
                      aria-label={t("widget.actions.duplicate")}
                      title={t("widget.actions.duplicate")}
                      disabled={duplicate.isPending}
                      onClick={() => duplicate.mutate(w.uuid)}
                    >
                      <Copy className="size-3.5" />
                    </Button>
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon"
                      className="size-7"
                      aria-label={t("widget.actions.delete")}
                      title={t("widget.actions.delete")}
                      disabled={remove.isPending}
                      onClick={async () => {
                        if (
                          await confirmDelete({
                            title: t("widget.confirm_delete"),
                          })
                        )
                          remove.mutate(w.uuid);
                      }}
                    >
                      <Trash2 className="size-3.5" />
                    </Button>
                  </>
                ) : null}
              </div>
            ))}
            {canManage ? (
              <form
                className="flex gap-2"
                onSubmit={(e) => {
                  e.preventDefault();
                  if (newName.trim()) create.mutate(newName.trim());
                }}
              >
                <Input
                  value={newName}
                  maxLength={120}
                  className="h-9 w-48"
                  placeholder={t("widget.new_placeholder")}
                  aria-label={t("widget.new_placeholder")}
                  onChange={(e) => setNewName(e.target.value)}
                />
                <Button
                  type="submit"
                  size="icon"
                  aria-label={t("widget.actions.create")}
                  disabled={!newName.trim() || create.isPending}
                >
                  <Plus className="size-4" />
                </Button>
              </form>
            ) : null}
          </div>

          {current ? (
            <WidgetEditor
              key={current.uuid}
              widget={current}
              canManage={canManage}
              canPublish={canPublish}
              onInstanceSave={saveInstance}
              flushRef={flushRef}
            />
          ) : (
            <Card>
              <CardContent className="text-muted-foreground py-10 text-center text-sm">
                {t("widget.empty_hint")}
              </CardContent>
            </Card>
          )}
        </div>
      ) : null}
    </div>
  );
}
