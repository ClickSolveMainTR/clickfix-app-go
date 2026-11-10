"use client";

import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { MessageSquareHeart } from "lucide-react";
import { useMemo, useState } from "react";

import {
  EntityDrawer,
  EntityTable,
  useServerListState,
} from "@/components/entity";
import { PageHeader } from "@/components/layout/page-header";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useLocale } from "@/providers/locale-provider";

import {
  platformGuidesService,
  type ProductFeedback,
} from "../services/guides.service";

const KINDS = ["bug", "idea", "other"] as const;
const STATUSES = ["new", "reviewed", "closed"] as const;
const ALL = "all";

/** Product feedback about ClickFix (platform.product_feedback.read). */
export function PlatformFeedbackPage() {
  const { t, locale } = useLocale();
  const qc = useQueryClient();
  const [kind, setKind] = useState<string>(ALL);
  const [status, setStatus] = useState<string>("new");
  const [openUuid, setOpenUuid] = useState<string | null>(null);
  const listState = useServerListState({ initialPageSize: 25 });
  const { limit, offset } = listState.params;
  const params = {
    kind: kind === ALL ? undefined : kind,
    status: status === ALL ? undefined : status,
    limit,
    offset,
  };
  const list = useQuery({
    queryKey: ["platform-product-feedback", params],
    queryFn: () => platformGuidesService.feedback(params),
    placeholderData: keepPreviousData,
  });
  const setStatusMutation = useMutation({
    mutationFn: (v: { uuid: string; status: ProductFeedback["status"] }) =>
      platformGuidesService.setFeedbackStatus(v.uuid, v.status),
    onSettled: () =>
      qc.invalidateQueries({ queryKey: ["platform-product-feedback"] }),
  });
  const items = list.data?.items ?? [];
  const open = items.find((f) => f.uuid === openUuid) ?? null;
  const toFirstPage = () =>
    listState.setPagination((p) => ({ ...p, pageIndex: 0 }));

  const { mutate: changeStatus, isPending: statusPending } = setStatusMutation;
  const pendingUuid = setStatusMutation.variables?.uuid;

  const columns = useMemo(
    () =>
      [
        createColumn<ProductFeedback>({
          accessorKey: "kind",
          labelKey: "guides.platform_feedback.columns.kind",
          cell: ({ row }) => (
            <Badge
              variant={row.original.kind === "bug" ? "danger" : "secondary"}
            >
              {t(`guides.feedback.kind.${row.original.kind}`)}
            </Badge>
          ),
        }),
        createColumn<ProductFeedback>({
          accessorKey: "message",
          labelKey: "guides.platform_feedback.columns.message",
          enableHiding: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <p className="line-clamp-2 max-w-md text-sm whitespace-pre-wrap">
              {row.original.message}
            </p>
          ),
        }),
        createColumn<ProductFeedback>({
          id: "organization",
          accessorKey: "organization_name",
          labelKey: "guides.platform_feedback.columns.organization",
          cell: ({ row }) => (
            <div className="min-w-0 text-xs">
              <div className="font-medium">
                {row.original.organization_name}
              </div>
              <div className="text-muted-foreground truncate">
                {row.original.user_email}
              </div>
            </div>
          ),
        }),
        createColumn<ProductFeedback>({
          id: "source",
          accessorFn: (f) => f.surface,
          labelKey: "guides.platform_feedback.columns.source",
          defaultHidden: true,
          cell: ({ row }) => (
            <span className="text-muted-foreground text-xs">
              {row.original.surface} · {row.original.route || "—"}
              {row.original.app_version ? ` · ${row.original.app_version}` : ""}
            </span>
          ),
        }),
        createColumn<ProductFeedback>({
          accessorKey: "created_at",
          labelKey: "guides.platform_feedback.columns.created",
          cell: ({ row }) => (
            <span className="text-muted-foreground text-xs whitespace-nowrap">
              {new Date(row.original.created_at).toLocaleString(locale)}
            </span>
          ),
        }),
        createColumn<ProductFeedback>({
          accessorKey: "status",
          labelKey: "guides.platform_feedback.columns.status",
          enableHiding: false,
          cell: ({ row }) => (
            <Select
              value={row.original.status}
              disabled={statusPending && pendingUuid === row.original.uuid}
              onValueChange={(v) =>
                changeStatus({
                  uuid: row.original.uuid,
                  status: v as ProductFeedback["status"],
                })
              }
            >
              <SelectTrigger
                className="h-8 w-36"
                aria-label={t("guides.platform_feedback.columns.status")}
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {STATUSES.map((s) => (
                  <SelectItem key={s} value={s}>
                    {t(`guides.platform_feedback.status.${s}`)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          ),
        }),
      ] as ColumnDef<ProductFeedback, unknown>[],
    [changeStatus, locale, pendingUuid, statusPending, t],
  );

  return (
    <div className="space-y-6">
      <PageHeader
        icon={<MessageSquareHeart className="size-7" />}
        title={t("guides.platform_feedback.title")}
        description={t("guides.platform_feedback.description")}
      />
      <EntityTable
        columns={columns}
        data={items}
        getRowId={(f) => f.uuid}
        onRowClick={(f) => setOpenUuid(f.uuid)}
        isLoading={list.isLoading}
        isError={list.isError}
        errorTitle={t("common.error_server")}
        onRetry={() => void list.refetch()}
        emptyTitle={t("guides.platform_feedback.empty")}
        emptyDescription=""
        total={list.data?.total ?? 0}
        state={listState.tableState}
        features={{
          // The endpoint takes `kind`, `status`, `limit` and `offset` only.
          sorting: false,
          globalFilter: false,
          columnFilters: false,
          rowSelection: false,
          viewMode: false,
        }}
        toolbarExtra={
          <div className="flex gap-2">
            <Select
              value={kind}
              onValueChange={(v) => {
                setKind(v);
                toFirstPage();
              }}
            >
              <SelectTrigger
                className="h-8 w-36"
                aria-label={t("guides.platform_feedback.columns.kind")}
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={ALL}>{t("guides.all")}</SelectItem>
                {KINDS.map((k) => (
                  <SelectItem key={k} value={k}>
                    {t(`guides.feedback.kind.${k}`)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Select
              value={status}
              onValueChange={(v) => {
                setStatus(v);
                toFirstPage();
              }}
            >
              <SelectTrigger
                className="h-8 w-36"
                aria-label={t("guides.platform_feedback.columns.status")}
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={ALL}>{t("guides.all")}</SelectItem>
                {STATUSES.map((s) => (
                  <SelectItem key={s} value={s}>
                    {t(`guides.platform_feedback.status.${s}`)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        }
      />
      <EntityDrawer
        open={Boolean(open)}
        onOpenChange={(next) => !next && setOpenUuid(null)}
        title={open ? t(`guides.feedback.kind.${open.kind}`) : ""}
        description={
          open
            ? `${open.organization_name} · ${new Date(open.created_at).toLocaleString(locale)}`
            : undefined
        }
      >
        {open ? (
          <div className="space-y-4 text-sm">
            <p className="whitespace-pre-wrap">{open.message}</p>
            <dl className="text-muted-foreground grid gap-1 text-xs">
              <div>{open.user_email}</div>
              <div>
                {open.surface} · {open.route || "—"}
                {open.app_version ? ` · ${open.app_version}` : ""}
              </div>
              {open.attachment_uuids.length ? (
                <div>
                  {t("guides.platform_feedback.attachments", {
                    count: open.attachment_uuids.length,
                  })}
                </div>
              ) : null}
            </dl>
          </div>
        ) : null}
      </EntityDrawer>
    </div>
  );
}
