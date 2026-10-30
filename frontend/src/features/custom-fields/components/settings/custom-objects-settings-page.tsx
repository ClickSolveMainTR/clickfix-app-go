"use client";

import {
  Archive,
  ArchiveRestore,
  ListChecks,
  Pencil,
  Rows3,
} from "lucide-react";
import Link from "next/link";
import { useState } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { EntityCreateButton, EntityPage } from "@/components/entity";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { cn } from "@/lib/utils";
import { useDialogs } from "@/providers/dialog-provider";
import { usePermission } from "@/providers/permission-provider";
import { useLocale } from "@/providers/locale-provider";

import { useSaveObjectType } from "../../hooks/use-custom-field-mutations";
import {
  useCustomFieldMeta,
  useCustomObjectTypes,
} from "../../hooks/use-custom-fields";
import { objectIcon } from "../../lib/object-icons";
import type { CustomObjectType } from "../../types";
import { objectFieldsTab } from "./custom-fields-settings-page";
import { ObjectTypeDialog } from "./object-type-dialog";

/** `/settings/custom-objects`: custom object types of the organization. */
export function CustomObjectsSettingsPage() {
  const { t } = useLocale();
  const { can } = usePermission();
  const { confirm } = useDialogs();
  const canManage = can(permissions.tenant.customFields.manage);
  const canReadRecords = can(permissions.tenant.customObjects.read);
  const query = useCustomObjectTypes();
  const meta = useCustomFieldMeta();
  const save = useSaveObjectType();
  const [showArchived, setShowArchived] = useState(false);
  const [dialog, setDialog] = useState<{
    open: boolean;
    objectType: CustomObjectType | null;
  }>({ open: false, objectType: null });

  const all = query.data ?? [];
  const archivedCount = all.filter((o) => o.archived).length;
  const visible = showArchived ? all : all.filter((o) => !o.archived);
  const limit = meta.data?.limits.object_types;
  const atLimit = Boolean(limit && all.length >= limit);

  const toggleArchive = async (o: CustomObjectType) => {
    if (!o.archived) {
      const ok = await confirm({
        title: t("custom_fields.objects.archive_title"),
        description: t("custom_fields.objects.archive_description", {
          name: o.plural_label,
        }),
        confirmLabel: t("custom_fields.actions.archive"),
        variant: "destructive",
      });
      if (!ok) return;
    }
    await save.mutateAsync({
      uuid: o.uuid,
      body: {
        singular_label: o.singular_label,
        plural_label: o.plural_label,
        description: o.description,
        icon: o.icon,
        archived: !o.archived,
      },
    });
  };

  return (
    <EntityPage
      title={t("custom_fields.objects.title")}
      description={t("custom_fields.objects.subtitle")}
      permission={permissions.tenant.customFields.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("custom_fields.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.cms.home },
        { label: t("layout.section_settings") },
        { label: t("custom_fields.objects.title") },
      ]}
      actions={
        canManage ? (
          <EntityCreateButton
            onClick={() => setDialog({ open: true, objectType: null })}
            label={t("custom_fields.objects.actions.create")}
          />
        ) : null
      }
    >
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          <Switch
            id="cot-show-archived"
            checked={showArchived}
            onCheckedChange={setShowArchived}
          />
          <Label htmlFor="cot-show-archived" className="font-normal">
            {t("custom_fields.show_archived", { count: archivedCount })}
          </Label>
        </div>
        {limit ? (
          <span
            className={cn(
              "text-muted-foreground text-xs tabular-nums",
              atLimit && "text-destructive",
            )}
          >
            {t("custom_fields.limit", { count: all.length, limit })}
          </span>
        ) : null}
      </div>

      {query.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : query.isLoading ? (
        <div className="grid gap-3 sm:grid-cols-2">
          {Array.from({ length: 2 }).map((_, i) => (
            <Skeleton key={i} className="h-28 w-full" />
          ))}
        </div>
      ) : visible.length === 0 ? (
        <EmptyState
          title={t("custom_fields.objects.empty_title")}
          description={t("custom_fields.objects.empty_description")}
        />
      ) : (
        <div className="grid gap-3 sm:grid-cols-2">
          {visible.map((o) => {
            const Icon = objectIcon(o.icon);
            return (
              <Card
                key={o.uuid}
                className={cn(
                  "flex flex-col gap-3 p-4",
                  o.archived && "bg-muted/40",
                )}
              >
                <div className="flex items-start gap-3">
                  <span className="bg-primary/10 text-primary flex size-9 shrink-0 items-center justify-center rounded-md">
                    <Icon className="size-4" />
                  </span>
                  <div className="min-w-0 flex-1">
                    <div className="flex flex-wrap items-center gap-2">
                      <span
                        className={cn(
                          "font-medium",
                          o.archived && "text-muted-foreground line-through",
                        )}
                      >
                        {o.plural_label}
                      </span>
                      {o.archived ? (
                        <Badge variant="secondary">
                          {t("custom_fields.archived")}
                        </Badge>
                      ) : null}
                    </div>
                    <div className="text-muted-foreground mt-0.5 text-xs">
                      <code>{o.key}</code>
                      <span aria-hidden> · </span>
                      {o.singular_label}
                    </div>
                    {o.description ? (
                      <p className="text-muted-foreground mt-1 line-clamp-2 text-sm">
                        {o.description}
                      </p>
                    ) : null}
                  </div>
                </div>
                <div className="flex flex-wrap items-center gap-1.5">
                  {!o.archived && canReadRecords ? (
                    <Button asChild size="sm" variant="outline">
                      <Link href={routes.cms.objects.records(o.uuid)}>
                        <Rows3 className="size-4" />
                        {t("custom_fields.objects.actions.records")}
                      </Link>
                    </Button>
                  ) : null}
                  {!o.archived ? (
                    <Button asChild size="sm" variant="outline">
                      <Link
                        href={routes.cms.settings.customFieldsTab(
                          objectFieldsTab(o.uuid),
                        )}
                      >
                        <ListChecks className="size-4" />
                        {t("custom_fields.objects.actions.fields")}
                      </Link>
                    </Button>
                  ) : null}
                  {canManage ? (
                    <div className="ms-auto flex items-center gap-1">
                      {!o.archived ? (
                        <Button
                          type="button"
                          size="icon-sm"
                          variant="ghost"
                          aria-label={t("custom_fields.actions.edit")}
                          title={t("custom_fields.actions.edit")}
                          onClick={() =>
                            setDialog({ open: true, objectType: o })
                          }
                        >
                          <Pencil className="size-4" />
                        </Button>
                      ) : null}
                      <Button
                        type="button"
                        size="icon-sm"
                        variant="ghost"
                        disabled={save.isPending}
                        aria-label={
                          o.archived
                            ? t("custom_fields.actions.restore")
                            : t("custom_fields.actions.archive")
                        }
                        title={
                          o.archived
                            ? t("custom_fields.actions.restore")
                            : t("custom_fields.actions.archive")
                        }
                        onClick={() => void toggleArchive(o)}
                      >
                        {o.archived ? (
                          <ArchiveRestore className="size-4" />
                        ) : (
                          <Archive className="size-4" />
                        )}
                      </Button>
                    </div>
                  ) : null}
                </div>
              </Card>
            );
          })}
        </div>
      )}

      {canManage ? (
        <ObjectTypeDialog
          open={dialog.open}
          onOpenChange={(open) => setDialog((d) => ({ ...d, open }))}
          objectType={dialog.objectType}
        />
      ) : null}
    </EntityPage>
  );
}
