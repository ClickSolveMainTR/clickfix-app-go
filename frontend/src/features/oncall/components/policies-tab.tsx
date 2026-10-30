"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Pencil, Plus, Siren, Trash2 } from "lucide-react";
import { useState } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { DeleteDialog } from "@/components/dialogs/delete-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { useMemberDirectory } from "@/features/tenant-members/hooks/use-member-directory";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

import {
  oncallService,
  type EscalationPolicy,
  type EscalationPolicyInput,
  type EscalationStep,
} from "../services/oncall.service";
import { useOncallError } from "../lib/use-oncall-error";
import { PolicyDialog } from "./policy-dialog";

export function PoliciesTab({
  slug,
  canWrite,
}: {
  slug: string;
  canWrite: boolean;
}) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const onError = useOncallError();
  const directory = useMemberDirectory();
  const key = ["tenant", slug, "oncall", "policies"] as const;
  const policies = useQuery({
    queryKey: key,
    queryFn: () => oncallService.policies(slug),
  });
  const rotas = useQuery({
    queryKey: ["tenant", slug, "oncall", "rotas"],
    queryFn: () => oncallService.rotas(slug),
  });
  const [editing, setEditing] = useState<EscalationPolicy | "new" | null>(null);
  const [deleting, setDeleting] = useState<EscalationPolicy | null>(null);

  const save = useMutation({
    mutationFn: (body: EscalationPolicyInput) =>
      editing && editing !== "new"
        ? oncallService.updatePolicy(slug, editing.uuid, body)
        : oncallService.createPolicy(slug, body),
    onSuccess: async () => {
      setEditing(null);
      await qc.invalidateQueries({ queryKey: key });
      appToast.success(t("oncall.toast.saved"));
    },
    onError,
  });
  const remove = useMutation({
    mutationFn: (uuid: string) => oncallService.removePolicy(slug, uuid),
    onSuccess: async () => {
      setDeleting(null);
      await qc.invalidateQueries({ queryKey: key });
      appToast.success(t("oncall.toast.deleted"));
    },
    onError,
  });

  if (policies.isLoading) return <Loading label={t("common.loading")} />;
  if (policies.isError)
    return (
      <ErrorState
        title={t("oncall.error.title")}
        description={t("oncall.error.description")}
        onRetry={() => void policies.refetch()}
      />
    );
  const items = policies.data ?? [];
  const rotaName = (uuid: string) =>
    rotas.data?.find((r) => r.uuid === uuid)?.name ?? uuid.slice(0, 8);
  const targetLabel = (s: EscalationStep) =>
    s.rota
      ? `${t("oncall.target.rota")}: ${rotaName(s.rota)}`
      : s.team
        ? `${t("oncall.target.team")}: ${directory.teamName(s.team)}`
        : `${t("oncall.target.agent")}: ${directory.memberName(s.agent)}`;

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <p className="text-muted-foreground text-sm">
          {t("oncall.policies.hint")}
        </p>
        {canWrite && (
          <Button size="sm" onClick={() => setEditing("new")}>
            <Plus className="size-4" />
            {t("oncall.policies.new")}
          </Button>
        )}
      </div>
      {items.length === 0 && (
        <EmptyState
          icon={<Siren className="size-6" />}
          title={t("oncall.policies.empty_title")}
          description={t("oncall.policies.empty_description")}
        />
      )}
      {items.map((p) => (
        <Card key={p.uuid}>
          <CardContent className="space-y-3 py-4">
            <div className="flex flex-wrap items-start gap-3">
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="font-medium">{p.name}</span>
                  {!p.enabled && (
                    <Badge variant="secondary">
                      {t("oncall.policies.disabled")}
                    </Badge>
                  )}
                  {p.incidents && (
                    <Badge variant="outline">
                      {t("oncall.policies.incidents")}
                    </Badge>
                  )}
                  {p.urgent_conversations && (
                    <Badge variant="outline">
                      {t("oncall.policies.urgent")}
                    </Badge>
                  )}
                </div>
              </div>
              {canWrite && (
                <div className="flex gap-1">
                  <Button
                    size="icon"
                    variant="ghost"
                    aria-label={t("oncall.policies.edit")}
                    onClick={() => setEditing(p)}
                  >
                    <Pencil className="size-4" />
                  </Button>
                  <Button
                    size="icon"
                    variant="ghost"
                    aria-label={t("oncall.policies.delete")}
                    onClick={() => setDeleting(p)}
                  >
                    <Trash2 className="size-4" />
                  </Button>
                </div>
              )}
            </div>
            <ol className="space-y-1.5">
              {p.steps.map((s, i) => (
                <li
                  key={i}
                  className="bg-muted/50 flex flex-wrap items-center gap-x-3 gap-y-1 rounded-md px-2.5 py-1.5 text-sm"
                >
                  <span className="text-muted-foreground w-24 shrink-0 text-xs tabular-nums">
                    {s.after_minutes === 0
                      ? t("oncall.policies.immediately")
                      : t("oncall.policies.after", {
                          minutes: s.after_minutes,
                        })}
                  </span>
                  <span className="min-w-0 flex-1 truncate">
                    {targetLabel(s)}
                  </span>
                  <span className="text-muted-foreground text-xs">
                    {s.channels.map((c) => t(`oncall.channel.${c}`)).join(", ")}
                  </span>
                </li>
              ))}
            </ol>
          </CardContent>
        </Card>
      ))}
      {editing && (
        <PolicyDialog
          policy={editing === "new" ? null : editing}
          rotas={rotas.data ?? []}
          pending={save.isPending}
          onOpenChange={(o) => !o && setEditing(null)}
          onSubmit={(body) => save.mutate(body)}
        />
      )}
      <DeleteDialog
        open={Boolean(deleting)}
        title={t("oncall.policies.delete_title", {
          name: deleting?.name ?? "",
        })}
        description={t("oncall.policies.delete_description")}
        isPending={remove.isPending}
        onConfirm={() => deleting && remove.mutate(deleting.uuid)}
        onCancel={() => setDeleting(null)}
      />
    </div>
  );
}
