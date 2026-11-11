"use client";

import { keepPreviousData, useQuery } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { Undo2 } from "lucide-react";

import { EntityTable, useServerListState } from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { datetime } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { useVisibility } from "../hooks/use-visibility";
import { forumKeys } from "../lib/keys";
import { forumService, type ForumRemovedPost } from "../services/forum.service";
import { VisibilityDialog } from "./visibility-dialog";

const PAGE = 20;

/**
 * Topics and replies taken down (CF-182), newest first, with who removed
 * each and why. "Publish again" asks first; a removed reply whose topic is
 * down is refused by the API (409) until the topic is back.
 */
export function RemovedPosts({ org }: { org: string }) {
  const { t, locale } = useLocale();
  const listState = useServerListState({ initialPageSize: PAGE });
  const { limit, offset } = listState.params;
  const list = useQuery({
    queryKey: forumKeys.removed(org, offset),
    queryFn: () => forumService.removed(limit, offset),
    placeholderData: keepPreviousData,
  });
  const visibility = useVisibility(org);

  // Rebuilt every render: the action cell reads the pending mutation.
  const columns = [
    createColumn<ForumRemovedPost>({
      id: "post",
      labelKey: "forum.removed.columns.post",
      enableHiding: false,
      gridPrimary: true,
      cell: ({ row: { original: p } }) => {
        const isReply = p.topic_id != null;
        return (
          <div className="max-w-xl min-w-0 space-y-1">
            <div className="flex flex-wrap items-center gap-1.5">
              <Badge variant={isReply ? "secondary" : "default"}>
                {isReply ? t("forum.kind.reply") : t("forum.kind.topic")}
              </Badge>
              {isReply && p.topic_title && (
                <span className="text-muted-foreground text-xs break-words">
                  {t("forum.removed.of_topic", { title: p.topic_title })}
                </span>
              )}
            </div>
            {p.title && <p className="font-medium break-words">{p.title}</p>}
            <p className="text-muted-foreground line-clamp-2 text-sm break-words">
              {p.body}
            </p>
          </div>
        );
      },
    }),
    createColumn<ForumRemovedPost>({
      id: "removed",
      labelKey: "forum.removed.columns.removed",
      cell: ({ row: { original: p } }) => (
        <div className="text-muted-foreground space-y-0.5 text-xs">
          <p className="whitespace-nowrap">
            {p.removed_at
              ? datetime(p.removed_at, "dd.MM.yyyy HH:mm", locale)
              : null}
          </p>
          {p.removed_by_name && <p>{p.removed_by_name}</p>}
        </div>
      ),
    }),
    createColumn<ForumRemovedPost>({
      accessorKey: "removed_reason",
      labelKey: "forum.removed.columns.reason",
      cell: ({ row: { original: p } }) => (
        <span className="text-muted-foreground line-clamp-2 text-xs break-words">
          {p.removed_reason || "—"}
        </span>
      ),
    }),
    createColumn<ForumRemovedPost>({
      id: "actions",
      labelKey: "common.actions",
      enableHiding: false,
      meta: {
        headerClassName: "text-right",
        cellClassName: "text-right whitespace-nowrap",
      },
      cell: ({ row: { original: p } }) => (
        <Button
          size="sm"
          variant="outline"
          disabled={visibility.isPending}
          onClick={() =>
            visibility.ask({
              action: "republish",
              kind: p.topic_id != null ? "reply" : "topic",
              uuid: p.uuid,
            })
          }
        >
          <Undo2 className="size-4" />
          {t("forum.visibility.republish")}
        </Button>
      ),
    }),
  ] as ColumnDef<ForumRemovedPost, unknown>[];

  return (
    <>
      <EntityTable
        columns={columns}
        data={list.data?.items ?? []}
        getRowId={(p) => p.uuid}
        isLoading={list.isLoading}
        isError={list.isError}
        errorTitle={t("forum.error.title")}
        errorDescription={t("forum.error.description")}
        onRetry={() => void list.refetch()}
        emptyTitle={t("forum.removed.empty")}
        emptyDescription=""
        total={list.data?.total ?? 0}
        state={listState.tableState}
        pageSizeOptions={[PAGE]}
        renderSubRow={(p) => (
          <div className="space-y-1 ps-10">
            {p.title && <p className="font-medium break-words">{p.title}</p>}
            <p className="text-sm break-words whitespace-pre-wrap">{p.body}</p>
          </div>
        )}
        features={{
          // The endpoint takes `limit` and `offset` only.
          toolbar: false,
          sorting: false,
          globalFilter: false,
          columnFilters: false,
          rowSelection: false,
          viewMode: false,
        }}
      />
      <VisibilityDialog
        request={visibility.pending}
        isPending={visibility.isPending}
        onConfirm={visibility.confirmPending}
        onCancel={visibility.cancel}
      />
    </>
  );
}
