"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { MessageSquareText, Pencil, Plus, Trash2 } from "lucide-react";
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
  type SystemPlaybook,
} from "../services/connect.service";
import { PlaybookDialog } from "./playbook-dialog";
import { useConnectError } from "./use-connect-error";

/** "AI'a nasıl çalışacağını anlat": plain-language playbooks. */
export function PlaybooksCard({
  slug,
  playbooks,
  concepts,
  actions,
  canManage,
}: {
  slug: string;
  playbooks: SystemPlaybook[];
  concepts: SystemConcept[];
  actions: SystemAction[];
  canManage: boolean;
}) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const onError = useConnectError();
  const [editing, setEditing] = useState<SystemPlaybook | "new" | null>(null);
  const [deleting, setDeleting] = useState<SystemPlaybook | null>(null);
  const label = (keys: string[], all: { key: string; label: string }[]) =>
    keys.map((k) => all.find((a) => a.key === k)?.label ?? k);
  const remove = useMutation({
    mutationFn: (intent: string) => connectService.deletePlaybook(slug, intent),
    onSuccess: async () => {
      setDeleting(null);
      await qc.invalidateQueries({ queryKey: connectKeys.overview(slug) });
      appToast.success(t("integrations_connect.playbooks.deleted"));
    },
    onError,
  });

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <MessageSquareText className="size-4" />
          {t("integrations_connect.playbooks.title")}
        </CardTitle>
        <CardDescription>
          {t("integrations_connect.playbooks.description")}
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        {playbooks.length === 0 && (
          <p className="text-muted-foreground text-sm">
            {t("integrations_connect.playbooks.empty")}
          </p>
        )}
        {playbooks.map((p) => (
          <div key={p.intent} className="rounded-md border p-3">
            <div className="flex flex-wrap items-center gap-2">
              <p className="flex-1 text-sm font-medium">
                {p.title}{" "}
                <code className="text-muted-foreground text-xs">
                  {p.intent}
                </code>
                {!p.enabled && (
                  <Badge variant="secondary" className="ml-2">
                    {t("integrations_connect.off")}
                  </Badge>
                )}
              </p>
              {canManage && (
                <div className="flex gap-1">
                  <Button
                    size="icon"
                    variant="ghost"
                    aria-label={t("integrations_connect.playbooks.edit")}
                    onClick={() => setEditing(p)}
                  >
                    <Pencil className="size-4" />
                  </Button>
                  <Button
                    size="icon"
                    variant="ghost"
                    aria-label={t("integrations_connect.remove")}
                    onClick={() => setDeleting(p)}
                  >
                    <Trash2 className="size-4" />
                  </Button>
                </div>
              )}
            </div>
            <p className="text-muted-foreground mt-1 line-clamp-3 text-sm whitespace-pre-line">
              {p.instructions}
            </p>
            <div className="mt-2 flex flex-wrap gap-1">
              {label(
                p.concept_keys,
                concepts.map((c) => ({ key: c.key, label: c.label })),
              ).map((l) => (
                <Badge key={`c-${l}`} variant="outline">
                  {l}
                </Badge>
              ))}
              {label(
                p.action_keys,
                actions.map((a) => ({ key: a.key, label: a.title })),
              ).map((l) => (
                <Badge key={`a-${l}`} variant="secondary">
                  {l}
                </Badge>
              ))}
            </div>
          </div>
        ))}
        {canManage && (
          <Button variant="outline" onClick={() => setEditing("new")}>
            <Plus className="size-4" />
            {t("integrations_connect.playbooks.new")}
          </Button>
        )}
      </CardContent>
      {editing && (
        <PlaybookDialog
          slug={slug}
          playbook={editing === "new" ? null : editing}
          concepts={concepts}
          actions={actions}
          onClose={() => setEditing(null)}
        />
      )}
      <DeleteDialog
        open={Boolean(deleting)}
        title={t("integrations_connect.playbooks.delete_title")}
        description={deleting?.title ?? ""}
        isPending={remove.isPending}
        onConfirm={() => deleting && remove.mutate(deleting.intent)}
        onCancel={() => setDeleting(null)}
      />
    </Card>
  );
}
