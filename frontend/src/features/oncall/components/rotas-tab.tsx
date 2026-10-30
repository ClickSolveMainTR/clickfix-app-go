"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { addDays, format, startOfDay } from "date-fns";
import { CalendarRange, Pencil, Plus, Trash2, UserRound } from "lucide-react";
import { useMemo, useState } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { DeleteDialog } from "@/components/dialogs/delete-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { useMemberDirectory } from "@/features/tenant-members/hooks/use-member-directory";
import { cn, datetime } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

import { shiftsOfDay } from "../lib/rota";
import {
  oncallService,
  type OncallRota,
  type OncallRotaInput,
} from "../services/oncall.service";
import { useOncallError } from "../lib/use-oncall-error";
import { RotaDialog } from "./rota-dialog";

const DAYS = 14;
/** Agent chips cycle through the chart palette. */
const TONES = [
  "bg-chart-1/20 border-chart-1/40",
  "bg-chart-2/20 border-chart-2/40",
  "bg-chart-3/20 border-chart-3/40",
  "bg-chart-4/20 border-chart-4/40",
  "bg-chart-5/20 border-chart-5/40",
];

export function RotasTab({
  slug,
  canWrite,
}: {
  slug: string;
  canWrite: boolean;
}) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const onError = useOncallError();
  const key = ["tenant", slug, "oncall", "rotas"] as const;
  const rotas = useQuery({
    queryKey: key,
    queryFn: () => oncallService.rotas(slug),
  });
  const [editing, setEditing] = useState<OncallRota | "new" | null>(null);
  const [deleting, setDeleting] = useState<OncallRota | null>(null);

  const save = useMutation({
    mutationFn: (body: OncallRotaInput) =>
      editing && editing !== "new"
        ? oncallService.updateRota(slug, editing.uuid, body)
        : oncallService.createRota(slug, body),
    onSuccess: async () => {
      setEditing(null);
      await qc.invalidateQueries({ queryKey: key });
      appToast.success(t("oncall.toast.saved"));
    },
    onError,
  });
  const remove = useMutation({
    mutationFn: (uuid: string) => oncallService.removeRota(slug, uuid),
    onSuccess: async () => {
      setDeleting(null);
      await qc.invalidateQueries({ queryKey: key });
      appToast.success(t("oncall.toast.deleted"));
    },
    onError,
  });

  if (rotas.isLoading) return <Loading label={t("common.loading")} />;
  if (rotas.isError)
    return (
      <ErrorState
        title={t("oncall.error.title")}
        description={t("oncall.error.description")}
        onRetry={() => void rotas.refetch()}
      />
    );
  const items = rotas.data ?? [];

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <p className="text-muted-foreground text-sm">
          {t("oncall.rotas.hint")}
        </p>
        {canWrite && (
          <Button size="sm" onClick={() => setEditing("new")}>
            <Plus className="size-4" />
            {t("oncall.rotas.new")}
          </Button>
        )}
      </div>
      {items.length === 0 && (
        <EmptyState
          icon={<CalendarRange className="size-6" />}
          title={t("oncall.rotas.empty_title")}
          description={t("oncall.rotas.empty_description")}
        />
      )}
      {items.map((rota) => (
        <RotaCard
          key={rota.uuid}
          slug={slug}
          rota={rota}
          canWrite={canWrite}
          onEdit={() => setEditing(rota)}
          onDelete={() => setDeleting(rota)}
        />
      ))}
      {editing && (
        <RotaDialog
          rota={editing === "new" ? null : editing}
          pending={save.isPending}
          onOpenChange={(o) => !o && setEditing(null)}
          onSubmit={(body) => save.mutate(body)}
        />
      )}
      <DeleteDialog
        open={Boolean(deleting)}
        title={t("oncall.rotas.delete_title", { name: deleting?.name ?? "" })}
        description={t("oncall.rotas.delete_description")}
        isPending={remove.isPending}
        onConfirm={() => deleting && remove.mutate(deleting.uuid)}
        onCancel={() => setDeleting(null)}
      />
    </div>
  );
}

function RotaCard({
  slug,
  rota,
  canWrite,
  onEdit,
  onDelete,
}: {
  slug: string;
  rota: OncallRota;
  canWrite: boolean;
  onEdit: () => void;
  onDelete: () => void;
}) {
  const { t, locale } = useLocale();
  const directory = useMemberDirectory();
  const current = useQuery({
    queryKey: [
      "tenant",
      slug,
      "oncall",
      "rotas",
      rota.uuid,
      "current",
      rota.updated_at,
    ],
    queryFn: () => oncallService.current(slug, rota.uuid),
    refetchInterval: 60_000,
  });

  const days = useMemo(() => {
    const first = startOfDay(new Date());
    return Array.from({ length: DAYS }, (_, i) => {
      const day = addDays(first, i);
      return { day, shifts: shiftsOfDay(rota.layers, rota.overrides, day) };
    });
  }, [rota.layers, rota.overrides]);
  const agents = useMemo(
    () => [...new Set(rota.layers.flatMap((l) => l.agents))],
    [rota.layers],
  );
  const tone = (uuid: string | null) =>
    uuid === null
      ? "bg-muted border-dashed"
      : TONES[Math.max(0, agents.indexOf(uuid)) % TONES.length];

  return (
    <Card>
      <CardContent className="space-y-3 py-4">
        <div className="flex flex-wrap items-start gap-3">
          <div className="min-w-0 flex-1">
            <div className="flex flex-wrap items-center gap-2">
              <span className="font-medium">{rota.name}</span>
              <Badge variant="outline">{rota.timezone}</Badge>
              <span className="text-muted-foreground text-xs">
                {t("oncall.rotas.summary", {
                  layers: rota.layers.length,
                  overrides: rota.overrides.length,
                })}
              </span>
            </div>
            <p className="mt-1 flex items-center gap-1.5 text-sm">
              <UserRound className="text-muted-foreground size-4" />
              {t("oncall.rotas.now")}:{" "}
              <span className="font-medium">
                {current.data
                  ? current.data.agents.length
                    ? current.data.agents.map(directory.memberName).join(", ")
                    : t("oncall.rotas.nobody")
                  : "…"}
              </span>
            </p>
          </div>
          {canWrite && (
            <div className="flex gap-1">
              <Button
                size="icon"
                variant="ghost"
                aria-label={t("oncall.rotas.edit")}
                onClick={onEdit}
              >
                <Pencil className="size-4" />
              </Button>
              <Button
                size="icon"
                variant="ghost"
                aria-label={t("oncall.rotas.delete")}
                onClick={onDelete}
              >
                <Trash2 className="size-4" />
              </Button>
            </div>
          )}
        </div>
        <div>
          <p className="text-muted-foreground mb-1.5 text-xs">
            {t("oncall.rotas.calendar_hint", { days: DAYS })}
          </p>
          <div className="grid grid-cols-2 gap-1.5 sm:grid-cols-4 lg:grid-cols-7">
            {days.map(({ day, shifts }) => (
              <div key={day.toISOString()} className="rounded-md border p-1.5">
                <div className="text-muted-foreground mb-1 text-[11px] font-medium">
                  {datetime(day, "EEE d MMM", locale)}
                </div>
                <ul className="space-y-1">
                  {shifts.map((s) => (
                    <li
                      key={s.start.toISOString()}
                      className={cn(
                        "rounded border px-1.5 py-0.5 text-[11px] leading-tight",
                        tone(s.agent),
                        s.override && "ring-primary/60 ring-1",
                      )}
                      title={
                        s.override
                          ? t("oncall.rotas.override_badge")
                          : undefined
                      }
                    >
                      <span className="tabular-nums">
                        {format(s.start, "HH:mm")}–
                        {format(s.end, "HH:mm") === "00:00"
                          ? "24:00"
                          : format(s.end, "HH:mm")}
                      </span>{" "}
                      <span className="block truncate font-medium">
                        {s.agent
                          ? directory.memberName(s.agent)
                          : t("oncall.rotas.nobody")}
                      </span>
                    </li>
                  ))}
                </ul>
              </div>
            ))}
          </div>
        </div>
      </CardContent>
    </Card>
  );
}
