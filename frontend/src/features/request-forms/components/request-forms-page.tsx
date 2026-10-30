"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ClipboardList, Pencil, Plus, Trash2 } from "lucide-react";
import { useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { PageHeader } from "@/components/layout/page-header";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Permission } from "@/config/permissions";
import { useActiveOrganization } from "@/features/organizations";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

import {
  requestFormsService,
  type RequestForm,
  type RequestFormInput,
} from "../services/request-forms.service";
import { FormDialog } from "./form-dialog";
import { useDialogs } from "@/providers/dialog-provider";

/**
 * Dynamic request forms (CF-52): start forms per channel built from system
 * fields and the organization's custom fields, with conditional and
 * "required before resolving" fields. Uses the custom field permissions.
 */
export function RequestFormsPage() {
  const { confirmDelete } = useDialogs();
  const { t } = useLocale();
  const qc = useQueryClient();
  const { active: org } = useActiveOrganization();
  const perms = org?.permissions ?? [];
  const canRead = perms.includes(Permission.TenantCustomFieldsRead);
  const canWrite = perms.includes(Permission.TenantCustomFieldsManage);
  const key = ["tenant", org?.slug, "request-forms"] as const;
  const forms = useQuery({
    queryKey: key,
    queryFn: requestFormsService.list,
    enabled: Boolean(org) && canRead,
  });
  const [editing, setEditing] = useState<RequestForm | "new" | null>(null);
  const onError = (error: unknown) =>
    appToast.error(
      isApiError(error) ? error.message : t("request_forms.toast.failed"),
    );
  const save = useMutation({
    mutationFn: (body: RequestFormInput) =>
      editing && editing !== "new"
        ? requestFormsService.update(editing.uuid, body)
        : requestFormsService.create(body),
    onSuccess: async () => {
      setEditing(null);
      await qc.invalidateQueries({ queryKey: key });
      appToast.success(t("request_forms.toast.saved"));
    },
    onError,
  });
  const remove = useMutation({
    mutationFn: (uuid: string) => requestFormsService.remove(uuid),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: key });
      appToast.success(t("request_forms.toast.deleted"));
    },
    onError,
  });

  return (
    <div className="space-y-6">
      <PageHeader
        icon={<ClipboardList className="size-7" />}
        title={t("request_forms.title")}
        description={t("request_forms.description")}
        actions={
          canWrite ? (
            <Button size="sm" onClick={() => setEditing("new")}>
              <Plus className="size-4" />
              {t("request_forms.new")}
            </Button>
          ) : undefined
        }
      />
      {org && !canRead && (
        <p className="text-muted-foreground text-sm">
          {t("request_forms.forbidden")}
        </p>
      )}
      {forms.isLoading && <Loading label={t("common.loading")} />}
      {forms.isError && (
        <ErrorState
          title={t("request_forms.error.title")}
          description={t("request_forms.error.description")}
          onRetry={() => void forms.refetch()}
        />
      )}
      {forms.data?.length === 0 && (
        <Card>
          <CardContent className="text-muted-foreground py-8 text-center text-sm">
            {t("request_forms.empty")}
          </CardContent>
        </Card>
      )}
      <div className="space-y-3">
        {(forms.data ?? []).map((f) => (
          <Card key={f.uuid}>
            <CardContent className="flex items-start gap-3 py-4">
              <div className="min-w-0 flex-1 space-y-1">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="font-medium">{f.name}</span>
                  {!f.is_active && (
                    <Badge variant="secondary">
                      {t("request_forms.inactive")}
                    </Badge>
                  )}
                  {f.channels.map((c) => (
                    <Badge key={c} variant="outline">
                      {t(`request_forms.channel.${c}`)}
                    </Badge>
                  ))}
                </div>
                {f.description && (
                  <p className="text-muted-foreground text-sm">
                    {f.description}
                  </p>
                )}
                <p className="text-muted-foreground text-xs">
                  {t("request_forms.field_count", { count: f.fields.length })}
                </p>
              </div>
              {canWrite && (
                <div className="flex gap-1">
                  <Button
                    size="icon"
                    variant="ghost"
                    aria-label={t("request_forms.edit")}
                    onClick={() => setEditing(f)}
                  >
                    <Pencil className="size-4" />
                  </Button>
                  <Button
                    disabled={remove.isPending}
                    size="icon"
                    variant="ghost"
                    aria-label={t("request_forms.delete")}
                    onClick={async () => {
                      if (
                        await confirmDelete({
                          title: t("request_forms.confirm_delete"),
                        })
                      )
                        remove.mutate(f.uuid);
                    }}
                  >
                    <Trash2 className="size-4" />
                  </Button>
                </div>
              )}
            </CardContent>
          </Card>
        ))}
      </div>
      {editing && (
        <FormDialog
          open
          onOpenChange={(o) => !o && setEditing(null)}
          form={editing === "new" ? null : editing}
          pending={save.isPending}
          onSubmit={(body) => save.mutate(body)}
        />
      )}
    </div>
  );
}
