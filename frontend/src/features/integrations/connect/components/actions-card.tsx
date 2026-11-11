"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Pencil, Plus, Trash2, Zap } from "lucide-react";
import { useState } from "react";

import { DeleteDialog } from "@/components/dialogs/delete-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

import {
  connectKeys,
  connectService,
  type SystemAction,
  type SystemConcept,
} from "../services/connect.service";
import { ActionDialog } from "./action-dialog";
import { useConnectError } from "./use-connect-error";

/**
 * "Yapılabilecek işler". Each action is offered to the assistant as a
 * confirmation card and to MCP clients as `integrations_actions_run`.
 */
export function ActionsCard({
  slug,
  actions,
  concepts,
  canManage,
}: {
  slug: string;
  actions: SystemAction[];
  concepts: SystemConcept[];
  canManage: boolean;
}) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const onError = useConnectError();
  const [editing, setEditing] = useState<SystemAction | "new" | null>(null);
  const [deleting, setDeleting] = useState<SystemAction | null>(null);
  const remove = useMutation({
    mutationFn: (key: string) => connectService.deleteAction(slug, key),
    onSuccess: async () => {
      setDeleting(null);
      await qc.invalidateQueries({ queryKey: connectKeys.overview(slug) });
      appToast.success(t("integrations_connect.actions.deleted"));
    },
    onError,
  });

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Zap className="size-4" />
          {t("integrations_connect.actions.title")}
        </CardTitle>
        <CardDescription>
          {t("integrations_connect.actions.description")}
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        {actions.length === 0 && (
          <p className="text-muted-foreground text-sm">
            {t("integrations_connect.actions.empty")}
          </p>
        )}
        {actions.map((a) => (
          <div
            key={a.key}
            className="flex flex-wrap items-center gap-2 rounded-md border p-3"
          >
            <div className="min-w-0 flex-1">
              <p className="flex flex-wrap items-center gap-2 text-sm font-medium">
                {a.title}
                <code className="text-muted-foreground text-xs">{a.key}</code>
                {!a.enabled && (
                  <Badge variant="secondary">
                    {t("integrations_connect.off")}
                  </Badge>
                )}
              </p>
              <p className="text-muted-foreground font-mono text-xs break-all">
                {a.method} {a.path_template}
              </p>
              <div className="mt-1 flex flex-wrap gap-1">
                {a.allowed_for.map((c) => (
                  <Badge key={c} variant="outline">
                    {t(`integrations_connect.channel.${c}`)}
                  </Badge>
                ))}
                <Badge variant={a.requires_approval ? "warning" : "secondary"}>
                  {t(
                    a.requires_approval
                      ? "integrations_connect.actions.approval_on"
                      : "integrations_connect.actions.approval_off",
                  )}
                </Badge>
              </div>
            </div>
            {canManage && (
              <div className="flex gap-1">
                <Button
                  size="icon"
                  variant="ghost"
                  aria-label={t("integrations_connect.actions.edit")}
                  onClick={() => setEditing(a)}
                >
                  <Pencil className="size-4" />
                </Button>
                <Button
                  size="icon"
                  variant="ghost"
                  aria-label={t("integrations_connect.remove")}
                  onClick={() => setDeleting(a)}
                >
                  <Trash2 className="size-4" />
                </Button>
              </div>
            )}
          </div>
        ))}
        {canManage && (
          <Button variant="outline" onClick={() => setEditing("new")}>
            <Plus className="size-4" />
            {t("integrations_connect.actions.new")}
          </Button>
        )}
        <p className="text-muted-foreground text-xs">
          {t("integrations_connect.actions.surfaces")}
        </p>
      </CardContent>
      {editing && (
        <ActionDialog
          slug={slug}
          action={editing === "new" ? null : editing}
          concepts={concepts}
          onClose={() => setEditing(null)}
        />
      )}
      <DeleteDialog
        open={Boolean(deleting)}
        title={t("integrations_connect.actions.delete_title")}
        description={deleting?.title ?? ""}
        isPending={remove.isPending}
        onConfirm={() => deleting && remove.mutate(deleting.key)}
        onCancel={() => setDeleting(null)}
      />
    </Card>
  );
}
