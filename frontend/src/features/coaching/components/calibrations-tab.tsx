"use client";

import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { Plus } from "lucide-react";
import { useMemo, useState } from "react";

import { EntityTable, useServerListState } from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

import { formatDate } from "../lib/format";
import {
  coachingService,
  type CalibrationInput,
  type CalibrationSummary,
} from "../services/coaching.service";
import { CalibrationCreateDialog } from "./calibration-create-dialog";
import { CalibrationDialog } from "./calibration-dialog";

/** QA calibration sessions (CF-109), newest first. */
export function CalibrationsTab({ slug }: { slug: string }) {
  const { t, locale } = useLocale();
  const queryClient = useQueryClient();
  const listState = useServerListState({ initialPageSize: 20 });
  const { limit, offset } = listState.params;
  const [creating, setCreating] = useState(false);
  const [openUuid, setOpenUuid] = useState<string | null>(null);

  const list = useQuery({
    queryKey: [
      "tenant",
      slug,
      "quality",
      "calibrations",
      "list",
      limit,
      offset,
    ],
    queryFn: () => coachingService.calibrations(slug, { limit, offset }),
    placeholderData: keepPreviousData,
  });

  const columns = useMemo(
    () =>
      [
        createColumn<CalibrationSummary>({
          accessorKey: "name",
          labelKey: "coaching.calibrations.name",
          enableHiding: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="block max-w-[18rem] truncate font-medium">
              {row.original.name}
            </span>
          ),
        }),
        createColumn<CalibrationSummary>({
          id: "status",
          accessorFn: (c) => (c.closed_at ? "closed" : "open"),
          labelKey: "coaching.calibrations.status",
          cell: ({ row }) => (
            <Badge variant={row.original.closed_at ? "secondary" : "success"}>
              {row.original.closed_at
                ? t("coaching.calibrations.status_closed")
                : t("coaching.calibrations.status_open")}
            </Badge>
          ),
        }),
        createColumn<CalibrationSummary>({
          accessorKey: "created_at",
          labelKey: "coaching.calibrations.created_at",
          cell: ({ row }) => (
            <span className="text-muted-foreground whitespace-nowrap">
              {formatDate(row.original.created_at, locale)}
            </span>
          ),
        }),
      ] as ColumnDef<CalibrationSummary, unknown>[],
    [locale, t],
  );

  const create = useMutation({
    mutationFn: (body: CalibrationInput) =>
      coachingService.createCalibration(slug, body),
    onSuccess: (session) => {
      void queryClient.invalidateQueries({
        queryKey: ["tenant", slug, "quality", "calibrations"],
      });
      setCreating(false);
      listState.setPagination((p) => ({ ...p, pageIndex: 0 }));
      setOpenUuid(session.uuid);
      appToast.success(t("coaching.calibrations.created"));
    },
  });

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="text-muted-foreground max-w-2xl text-sm">
          {t("coaching.calibrations.description")}
        </p>
        <Button onClick={() => setCreating(true)}>
          <Plus className="size-4" />
          {t("coaching.calibrations.new")}
        </Button>
      </div>

      <EntityTable
        columns={columns}
        data={list.data?.items ?? []}
        getRowId={(c) => c.uuid}
        onRowClick={(c) => setOpenUuid(c.uuid)}
        isLoading={list.isLoading}
        isError={list.isError}
        errorTitle={t("coaching.error.title")}
        errorDescription={
          isApiError(list.error)
            ? list.error.message
            : t("coaching.error.description")
        }
        onRetry={() => void list.refetch()}
        emptyTitle={t("coaching.calibrations.empty_title")}
        emptyDescription={t("coaching.calibrations.empty")}
        total={list.data?.total ?? 0}
        state={listState.tableState}
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

      {creating && (
        <CalibrationCreateDialog
          slug={slug}
          pending={create.isPending}
          onOpenChange={setCreating}
          onSubmit={(body) => create.mutate(body)}
        />
      )}
      {openUuid && (
        <CalibrationDialog
          slug={slug}
          uuid={openUuid}
          onOpenChange={(open) => !open && setOpenUuid(null)}
        />
      )}
    </div>
  );
}
