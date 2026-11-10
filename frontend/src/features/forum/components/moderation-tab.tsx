"use client";

import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { Ban, Check, ShieldAlert, Sparkles, X } from "lucide-react";
import { EntityTable, useServerListState } from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { isApiError } from "@/lib/api";
import { datetime } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

import { forumKeys } from "../lib/keys";
import {
  forumService,
  type ForumModerationItem,
} from "../services/forum.service";

const PAGE = 20;
const KNOWN_FLAGS = new Set([
  "unsafe_instructions",
  "link_spam",
  "awaiting_review",
  "ai_reviewed",
]);

/**
 * Pending posts (CF-78): every visitor topic and reply waits here. Approve
 * publishes; reject and "spam" both hide it (the API has two outcomes:
 * published / rejected). A post is decided once: the API answers 409 when
 * someone else already handled it. AI triage is explicit and only adds a
 * suggestion.
 */
export function ModerationTab({ org }: { org: string }) {
  const { t, locale } = useLocale();
  const qc = useQueryClient();
  const listState = useServerListState({ initialPageSize: PAGE });
  const { limit, offset } = listState.params;
  const list = useQuery({
    // The page size is fixed (`pageSizeOptions`), so the offset keys the page.
    queryKey: forumKeys.moderation(org, offset),
    queryFn: () => forumService.moderation(limit, offset),
    placeholderData: keepPreviousData,
  });
  const invalidate = () =>
    qc.invalidateQueries({ queryKey: forumKeys.all(org) });
  const onError = (error: unknown) =>
    appToast.error(isApiError(error) ? error.message : t("forum.toast.failed"));

  const moderate = useMutation({
    mutationFn: (v: {
      uuid: string;
      status: "published" | "rejected";
      spam?: boolean;
      reply?: boolean;
    }) => forumService.moderate(v.uuid, v.status),
    onSuccess: async (_d, v) => {
      await invalidate();
      appToast.success(
        t(
          v.status === "published"
            ? "forum.toast.approved"
            : v.spam
              ? "forum.toast.spam"
              : "forum.toast.rejected",
        ),
      );
    },
    // 409: the post is no longer pending (another moderator decided it) or,
    // for a reply being published, its topic is not published yet. The
    // queue on screen is stale either way, so reload it.
    onError: async (error, v) => {
      if (!isApiError(error) || error.status !== 409) return onError(error);
      await invalidate();
      appToast.warning(
        t(
          v.status === "published" && v.reply
            ? "forum.toast.reply_conflict"
            : "forum.toast.already_moderated",
        ),
      );
    },
  });
  const assist = useMutation({
    mutationFn: (uuid: string) => forumService.assist(uuid),
    onSuccess: async (r) => {
      await invalidate();
      appToast.info(
        r.flagged
          ? t("forum.toast.ai_flagged", { reason: r.reason })
          : t("forum.toast.ai_clean"),
      );
    },
    onError: (error) =>
      appToast.error(
        isApiError(error) && error.status === 429
          ? t("forum.toast.ai_limited")
          : isApiError(error)
            ? error.message
            : t("forum.toast.failed"),
      ),
  });

  const flagLabel = (reason: string) => {
    if (!reason) return null;
    if (KNOWN_FLAGS.has(reason)) return t(`forum.flag.${reason}`);
    return reason.startsWith("ai: ")
      ? t("forum.flag.ai", { reason: reason.slice(4) })
      : reason;
  };
  const busy = (uuid: string) =>
    (moderate.isPending && moderate.variables?.uuid === uuid) ||
    (assist.isPending && assist.variables === uuid);

  // Rebuilt every render: the cells read the pending mutation (`busy`).
  const columns = [
    createColumn<ForumModerationItem>({
      id: "kind",
      labelKey: "forum.moderation.columns.kind",
      cell: ({ row: { original: p } }) => {
        const isReply = p.topic_id != null;
        const flag = flagLabel(p.flag_reason);
        const risky =
          p.flag_reason === "unsafe_instructions" ||
          p.flag_reason === "link_spam" ||
          p.flag_reason.startsWith("ai: ");
        return (
          <div className="flex flex-wrap items-center gap-1.5">
            <Badge variant={isReply ? "secondary" : "default"}>
              {isReply ? t("forum.kind.reply") : t("forum.kind.topic")}
            </Badge>
            {flag && (
              <Badge
                variant="outline"
                className={
                  risky
                    ? "border-red-500/40 text-red-700 dark:text-red-300"
                    : undefined
                }
              >
                {risky && <ShieldAlert className="size-3" />}
                {flag}
              </Badge>
            )}
          </div>
        );
      },
    }),
    createColumn<ForumModerationItem>({
      id: "post",
      labelKey: "forum.moderation.columns.post",
      enableHiding: false,
      gridPrimary: true,
      cell: ({ row: { original: p } }) => (
        <div className="max-w-xl min-w-0">
          {p.title && <p className="font-medium break-words">{p.title}</p>}
          <p className="text-muted-foreground line-clamp-2 text-sm break-words">
            {p.body}
          </p>
        </div>
      ),
    }),
    createColumn<ForumModerationItem>({
      accessorKey: "created_at",
      labelKey: "forum.moderation.columns.created",
      meta: {
        cellClassName: "text-muted-foreground text-xs whitespace-nowrap",
      },
      cell: ({ row: { original: p } }) =>
        datetime(p.created_at, "dd.MM.yyyy HH:mm", locale),
    }),
    createColumn<ForumModerationItem>({
      id: "actions",
      labelKey: "common.actions",
      enableHiding: false,
      meta: {
        headerClassName: "text-right",
        cellClassName: "text-right whitespace-nowrap",
      },
      cell: ({ row: { original: p } }) => (
        <div className="flex justify-end gap-1">
          <Button
            size="sm"
            disabled={busy(p.uuid)}
            onClick={() =>
              moderate.mutate({
                uuid: p.uuid,
                status: "published",
                reply: p.topic_id != null,
              })
            }
          >
            <Check className="size-4" />
            {t("forum.moderation.approve")}
          </Button>
          <Button
            size="icon"
            variant="outline"
            aria-label={t("forum.moderation.reject")}
            title={t("forum.moderation.reject")}
            disabled={busy(p.uuid)}
            onClick={() =>
              moderate.mutate({ uuid: p.uuid, status: "rejected" })
            }
          >
            <X className="size-4" />
          </Button>
          <Button
            size="icon"
            variant="outline"
            className="text-destructive"
            aria-label={t("forum.moderation.spam")}
            title={t("forum.moderation.spam")}
            disabled={busy(p.uuid)}
            onClick={() =>
              moderate.mutate({
                uuid: p.uuid,
                status: "rejected",
                spam: true,
              })
            }
          >
            <Ban className="size-4" />
          </Button>
          <Button
            size="icon"
            variant="ghost"
            aria-label={t("forum.moderation.assist")}
            title={t("forum.moderation.assist_hint")}
            disabled={busy(p.uuid)}
            onClick={() => assist.mutate(p.uuid)}
          >
            <Sparkles className="size-4" />
          </Button>
        </div>
      ),
    }),
  ] as ColumnDef<ForumModerationItem, unknown>[];

  return (
    <div className="space-y-3">
      {list.data ? (
        <p className="text-muted-foreground text-sm">
          {t("forum.moderation.count", { count: list.data.total })}
        </p>
      ) : null}
      <EntityTable
        columns={columns}
        data={list.data?.items ?? []}
        getRowId={(p) => p.uuid}
        isLoading={list.isLoading}
        isError={list.isError}
        errorTitle={t("forum.error.title")}
        errorDescription={t("forum.error.description")}
        onRetry={() => void list.refetch()}
        emptyTitle={t("forum.moderation.empty")}
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
    </div>
  );
}
