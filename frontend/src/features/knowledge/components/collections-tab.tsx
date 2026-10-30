"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { FolderTree, Pencil, Plus, Trash2 } from "lucide-react";
import { useMemo, useState } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { routes } from "@/config/routes";
import { useActiveOrganization } from "@/features/organizations";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

import { collectionTree } from "../lib/collections";
import {
  knowledgeKeys,
  knowledgeService,
  type KnowledgeCollection,
  type KnowledgeCollectionInput,
} from "../services/knowledge.service";
import { CollectionDialog } from "./collection-dialog";

type Editing =
  | { mode: "edit"; collection: KnowledgeCollection }
  | { mode: "new"; kind: "collection" | "category"; parent?: string };

export function CollectionsTab({ canWrite }: { canWrite: boolean }) {
  const { t, locale } = useLocale();
  const qc = useQueryClient();
  const { active: org } = useActiveOrganization();
  const key = knowledgeKeys.collections(org?.slug);
  const collections = useQuery({
    queryKey: key,
    queryFn: knowledgeService.collections,
    enabled: Boolean(org),
  });
  const tree = useMemo(
    () => collectionTree(collections.data ?? [], locale),
    [collections.data, locale],
  );
  const parents = tree.filter((n) => n.kind === "collection");
  const [editing, setEditing] = useState<Editing | null>(null);
  const [deleting, setDeleting] = useState<KnowledgeCollection | null>(null);

  const conflictMessage = (error: unknown) =>
    isApiError(error) && error.status === 409
      ? t("knowledge.collections.conflict")
      : isApiError(error) && error.status === 422
        ? t("knowledge.collections.invalid")
        : t("knowledge.toast.failed");

  const save = useMutation({
    mutationFn: (body: KnowledgeCollectionInput) =>
      editing?.mode === "edit"
        ? knowledgeService.updateCollection(editing.collection.uuid, body)
        : knowledgeService.createCollection(body),
    onSuccess: async () => {
      setEditing(null);
      await qc.invalidateQueries({ queryKey: knowledgeKeys.all(org?.slug) });
      appToast.success(t("knowledge.collections.saved"));
    },
    onError: (error) => appToast.error(conflictMessage(error)),
  });
  const remove = useMutation({
    mutationFn: (uuid: string) => knowledgeService.removeCollection(uuid),
    onSuccess: async () => {
      setDeleting(null);
      await qc.invalidateQueries({ queryKey: knowledgeKeys.all(org?.slug) });
      appToast.success(t("knowledge.collections.deleted"));
    },
    onError: (error) => {
      setDeleting(null);
      appToast.error(
        isApiError(error) && error.status === 409
          ? t("knowledge.collections.delete_conflict")
          : t("knowledge.toast.failed"),
      );
    },
  });

  if (collections.isLoading) return <Loading label={t("common.loading")} />;
  if (collections.isError) {
    return (
      <ErrorState
        title={t("knowledge.error.title")}
        description={t("knowledge.error.description")}
        onRetry={() => void collections.refetch()}
      />
    );
  }

  const actions = (c: KnowledgeCollection) =>
    canWrite ? (
      <div className="flex shrink-0 gap-1">
        {c.kind === "collection" && (
          <Button
            size="sm"
            variant="ghost"
            onClick={() =>
              setEditing({ mode: "new", kind: "category", parent: c.uuid })
            }
          >
            <Plus className="size-4" />
            <span className="hidden sm:inline">
              {t("knowledge.collections.add_category")}
            </span>
            <span className="sr-only sm:hidden">
              {t("knowledge.collections.add_category")}
            </span>
          </Button>
        )}
        <Button
          size="icon"
          variant="ghost"
          aria-label={t("knowledge.collections.edit", { name: c.name })}
          onClick={() => setEditing({ mode: "edit", collection: c })}
        >
          <Pencil className="size-4" />
        </Button>
        <Button
          size="icon"
          variant="ghost"
          aria-label={t("knowledge.collections.delete", { name: c.name })}
          onClick={() => setDeleting(c)}
        >
          <Trash2 className="size-4" />
        </Button>
      </div>
    ) : null;

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="text-muted-foreground max-w-2xl text-sm">
          {t("knowledge.collections.description")}
        </p>
        {canWrite && (
          <Button
            size="sm"
            onClick={() => setEditing({ mode: "new", kind: "collection" })}
          >
            <Plus className="size-4" />
            {t("knowledge.collections.new_collection")}
          </Button>
        )}
      </div>

      {tree.length === 0 ? (
        <EmptyState
          icon={<FolderTree className="text-signal size-5" />}
          title={t("knowledge.collections.empty_title")}
          description={t("knowledge.collections.empty_description")}
        />
      ) : (
        <div className="space-y-3">
          {tree.map((node) => (
            <Card key={node.uuid} className="py-0">
              <CardContent className="p-0">
                <div className="flex items-center gap-3 px-4 py-3">
                  <div className="min-w-0 flex-1">
                    <p className="truncate font-medium">{node.name}</p>
                    <p className="text-muted-foreground truncate font-mono text-xs">
                      {routes.help.collection(org?.slug ?? "", node.slug)}
                    </p>
                  </div>
                  {actions(node)}
                </div>
                {node.children.length > 0 && (
                  <ul className="border-border divide-border divide-y border-t">
                    {node.children.map((child) => (
                      <li
                        key={child.uuid}
                        className="flex items-center gap-3 py-2.5 pr-4 pl-8"
                      >
                        <div className="min-w-0 flex-1">
                          <p className="truncate text-sm">{child.name}</p>
                          <p className="text-muted-foreground truncate font-mono text-xs">
                            {child.slug}
                          </p>
                        </div>
                        {actions(child)}
                      </li>
                    ))}
                  </ul>
                )}
              </CardContent>
            </Card>
          ))}
        </div>
      )}

      {editing && (
        <CollectionDialog
          collection={editing.mode === "edit" ? editing.collection : null}
          kind={
            editing.mode === "edit"
              ? (editing.collection.kind as "collection" | "category")
              : editing.kind
          }
          parentUuid={editing.mode === "new" ? editing.parent : null}
          parents={parents}
          pending={save.isPending}
          onSubmit={(body) => save.mutate(body)}
          onOpenChange={(open) => !open && setEditing(null)}
        />
      )}
      <ConfirmDialog
        open={deleting !== null}
        title={t("knowledge.collections.confirm_delete_title", {
          name: deleting?.name ?? "",
        })}
        description={t("knowledge.collections.confirm_delete_description")}
        confirmLabel={t("knowledge.delete")}
        variant="destructive"
        isPending={remove.isPending}
        onConfirm={() => deleting && remove.mutate(deleting.uuid)}
        onCancel={() => setDeleting(null)}
      />
    </div>
  );
}
