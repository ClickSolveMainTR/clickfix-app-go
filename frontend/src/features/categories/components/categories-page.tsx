"use client";

import {
  Archive,
  ArchiveRestore,
  Download,
  Pencil,
  Plus,
  Trash2,
} from "lucide-react";
import { useMemo, useState } from "react";

import { Can } from "@/components/common/can";
import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { EntityDeleteDialog, EntityPage } from "@/components/entity";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Switch } from "@/components/ui/switch";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { CategoryFormDialog } from "@/features/categories/components/category-form-dialog";
import { CategoryReviewQueue } from "@/features/categories/components/category-review-queue";
import {
  useCategories,
  useDeleteCategory,
  useExportLabels,
  useSaveCategory,
} from "@/features/categories/hooks/use-categories";
import { buildTree } from "@/features/categories/lib";
import type { CategoryNode } from "@/features/categories/types";
import { usePermission } from "@/providers/permission-provider";
import { useLocale } from "@/providers/locale-provider";

type FormState = { category?: CategoryNode; parentUuid?: string } | null;

/**
 * Category taxonomy (CF-218): the organization-wide tree with department
 * branches, the review queue of records the classifier was unsure about, and
 * the label export (training set of the organization's own model, CF-219).
 */
export function CategoriesPage() {
  const { t } = useLocale();
  const { can } = usePermission();
  const [showArchived, setShowArchived] = useState(false);
  const [form, setForm] = useState<FormState>(null);
  const [deleteTarget, setDeleteTarget] = useState<CategoryNode | null>(null);
  const list = useCategories(showArchived);
  const remove = useDeleteCategory();
  const save = useSaveCategory();
  const exportLabels = useExportLabels();
  const tree = useMemo(() => buildTree(list.data?.items ?? []), [list.data]);
  const canManage = can(permissions.tenant.categories.manage);
  const canLabel = can(permissions.tenant.categories.label);

  const row = (node: CategoryNode) => (
    <li key={node.uuid}>
      <div
        className="hover:bg-muted/50 flex items-center gap-3 rounded-md px-2 py-2"
        style={{ paddingLeft: `${node.depth * 1.5 + 0.5}rem` }}
      >
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <span className="truncate font-medium">{node.name}</span>
            <span className="text-muted-foreground font-mono text-xs">
              {node.slug}
            </span>
            {node.department_uuid ? (
              <Badge variant="secondary">{t("categories.department")}</Badge>
            ) : null}
            {node.archived ? (
              <Badge variant="outline">{t("categories.archived")}</Badge>
            ) : null}
            {node.examples.length > 0 ? (
              <Badge variant="outline">
                {t("categories.examples_count", {
                  count: node.examples.length,
                })}
              </Badge>
            ) : null}
          </div>
          {node.description ? (
            <p className="text-muted-foreground truncate text-sm">
              {node.description}
            </p>
          ) : null}
        </div>
        {canManage ? (
          <div className="flex shrink-0 items-center gap-1">
            {!node.archived && node.depth < 4 ? (
              <Button
                size="icon-sm"
                variant="ghost"
                aria-label={t("categories.actions.add_child")}
                onClick={() => setForm({ parentUuid: node.uuid })}
              >
                <Plus />
              </Button>
            ) : null}
            <Button
              size="icon-sm"
              variant="ghost"
              aria-label={t("categories.actions.edit")}
              onClick={() => setForm({ category: node })}
            >
              <Pencil />
            </Button>
            <Button
              size="icon-sm"
              variant="ghost"
              aria-label={
                node.archived
                  ? t("categories.actions.unarchive")
                  : t("categories.actions.archive")
              }
              disabled={save.isPending}
              onClick={() =>
                save.mutate({
                  uuid: node.uuid,
                  body: { archived: !node.archived },
                })
              }
            >
              {node.archived ? <ArchiveRestore /> : <Archive />}
            </Button>
            <Button
              size="icon-sm"
              variant="ghost"
              aria-label={t("categories.actions.delete")}
              onClick={() => setDeleteTarget(node)}
            >
              <Trash2 />
            </Button>
          </div>
        ) : null}
      </div>
      {node.children.length > 0 ? <ul>{node.children.map(row)}</ul> : null}
    </li>
  );

  return (
    <EntityPage
      title={t("categories.title")}
      description={t("categories.description")}
      permission={permissions.tenant.categories.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("categories.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.cms.home },
        { label: t("categories.title") },
      ]}
      actions={
        <div className="flex items-center gap-2">
          <Can permission={permissions.tenant.categories.export}>
            <Button
              variant="outline"
              disabled={exportLabels.isPending}
              onClick={() => exportLabels.mutate()}
            >
              <Download />
              {t("categories.actions.export")}
            </Button>
          </Can>
          <Can permission={permissions.tenant.categories.manage}>
            <Button onClick={() => setForm({})}>
              <Plus />
              {t("categories.actions.create")}
            </Button>
          </Can>
        </div>
      }
    >
      <Tabs defaultValue="tree">
        <TabsList>
          <TabsTrigger value="tree">{t("categories.tabs.tree")}</TabsTrigger>
          {canLabel ? (
            <TabsTrigger value="review">
              {t("categories.tabs.review")}
            </TabsTrigger>
          ) : null}
        </TabsList>
        <TabsContent value="tree" className="space-y-3">
          <label className="text-muted-foreground flex items-center gap-2 text-sm">
            <Switch checked={showArchived} onCheckedChange={setShowArchived} />
            {t("categories.show_archived")}
          </label>
          {list.isError ? (
            <ErrorState
              title={t("categories.load_failed")}
              onRetry={() => void list.refetch()}
            />
          ) : !list.isLoading && tree.length === 0 ? (
            <EmptyState
              title={t("categories.empty_title")}
              description={t("categories.empty_description")}
            />
          ) : (
            <ul className="rounded-lg border p-2">{tree.map(row)}</ul>
          )}
        </TabsContent>
        {canLabel ? (
          <TabsContent value="review">
            <CategoryReviewQueue tree={tree} />
          </TabsContent>
        ) : null}
      </Tabs>

      <CategoryFormDialog
        open={form !== null}
        onOpenChange={(open) => !open && setForm(null)}
        tree={tree}
        category={form?.category}
        parentUuid={form?.parentUuid}
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
