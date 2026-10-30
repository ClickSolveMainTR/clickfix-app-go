"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Plus, Scale } from "lucide-react";
import { useState } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

import { formatDate } from "../lib/format";
import {
  coachingService,
  type CalibrationInput,
} from "../services/coaching.service";
import { CalibrationCreateDialog } from "./calibration-create-dialog";
import { CalibrationDialog } from "./calibration-dialog";

const PAGE = 20;

/** QA calibration sessions (CF-109), newest first. */
export function CalibrationsTab({ slug }: { slug: string }) {
  const { t, locale } = useLocale();
  const queryClient = useQueryClient();
  const [offset, setOffset] = useState(0);
  const [creating, setCreating] = useState(false);
  const [openUuid, setOpenUuid] = useState<string | null>(null);

  const list = useQuery({
    queryKey: ["tenant", slug, "quality", "calibrations", "list", offset],
    queryFn: () => coachingService.calibrations(slug, { limit: PAGE, offset }),
  });
  const items = list.data?.items ?? [];
  const total = list.data?.total ?? 0;

  const create = useMutation({
    mutationFn: (body: CalibrationInput) =>
      coachingService.createCalibration(slug, body),
    onSuccess: (session) => {
      void queryClient.invalidateQueries({
        queryKey: ["tenant", slug, "quality", "calibrations"],
      });
      setCreating(false);
      setOffset(0);
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

      {list.isLoading ? (
        <Loading label={t("common.loading")} />
      ) : list.isError ? (
        <ErrorState
          title={t("coaching.error.title")}
          description={
            isApiError(list.error)
              ? list.error.message
              : t("coaching.error.description")
          }
          onRetry={() => void list.refetch()}
        />
      ) : items.length === 0 ? (
        <EmptyState
          icon={<Scale className="size-6" />}
          title={t("coaching.calibrations.empty_title")}
          description={t("coaching.calibrations.empty")}
        />
      ) : (
        <div className="overflow-x-auto rounded-md border">
          <table className="w-full min-w-[30rem] text-sm">
            <thead className="bg-muted/50 text-muted-foreground text-left text-xs">
              <tr>
                <th scope="col" className="px-3 py-2 font-medium">
                  {t("coaching.calibrations.name")}
                </th>
                <th scope="col" className="px-3 py-2 font-medium">
                  {t("coaching.calibrations.status")}
                </th>
                <th scope="col" className="px-3 py-2 font-medium">
                  {t("coaching.calibrations.created_at")}
                </th>
              </tr>
            </thead>
            <tbody className="divide-y">
              {items.map((c) => (
                <tr key={c.uuid} className="hover:bg-muted/40">
                  <td className="px-3 py-2">
                    <button
                      type="button"
                      className="hover:text-primary max-w-[18rem] truncate text-left font-medium hover:underline"
                      onClick={() => setOpenUuid(c.uuid)}
                    >
                      {c.name}
                    </button>
                  </td>
                  <td className="px-3 py-2">
                    <Badge variant={c.closed_at ? "secondary" : "success"}>
                      {c.closed_at
                        ? t("coaching.calibrations.status_closed")
                        : t("coaching.calibrations.status_open")}
                    </Badge>
                  </td>
                  <td className="text-muted-foreground px-3 py-2 whitespace-nowrap">
                    {formatDate(c.created_at, locale)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {total > PAGE && (
        <div className="flex items-center justify-end gap-2 text-sm">
          <span className="text-muted-foreground">
            {t("coaching.pagination", {
              from: offset + 1,
              to: Math.min(offset + PAGE, total),
              total,
            })}
          </span>
          <Button
            size="sm"
            variant="outline"
            disabled={offset === 0}
            onClick={() => setOffset(Math.max(0, offset - PAGE))}
          >
            {t("coaching.previous")}
          </Button>
          <Button
            size="sm"
            variant="outline"
            disabled={offset + PAGE >= total}
            onClick={() => setOffset(offset + PAGE)}
          >
            {t("coaching.next")}
          </Button>
        </div>
      )}

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
