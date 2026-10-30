"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import { Permission } from "@/config/permissions";
import { CustomFieldInput, conditionHolds } from "@/features/custom-fields";
import { toApiValue, toInputValue } from "@/features/custom-fields/lib/values";
import { useActiveOrganization } from "@/features/organizations";
import { isApiError } from "@/lib/api";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

import {
  requestFormsService,
  type ConversationForm,
  type PublicRequestFormField,
} from "../services/request-forms.service";

/**
 * The request form a conversation started with (CF-52), in the inbox
 * context pane: agents fill its conversation fields; fields required before
 * resolving are flagged until they have a value.
 */
export function ConversationFormPanel({
  conversationUuid,
}: {
  conversationUuid: string;
}) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const { active: org } = useActiveOrganization();
  const perms = org?.permissions ?? [];
  const canEdit = perms.includes(Permission.TenantConversationsManage);
  const key = ["tenant", org?.slug, "conversation-form", conversationUuid];
  const { data } = useQuery({
    queryKey: key,
    queryFn: () => requestFormsService.conversation(conversationUuid),
    enabled: Boolean(org) && perms.includes(Permission.TenantConversationsRead),
  });
  if (!data?.form) return null;
  return (
    <FormValues
      key={conversationUuid}
      data={data}
      canEdit={canEdit}
      onSaved={(next) => qc.setQueryData(key, next)}
      conversationUuid={conversationUuid}
      t={t}
    />
  );
}

function FormValues({
  data,
  canEdit,
  onSaved,
  conversationUuid,
  t,
}: {
  data: ConversationForm;
  canEdit: boolean;
  onSaved: (next: ConversationForm) => void;
  conversationUuid: string;
  t: (key: string, params?: Record<string, string | number>) => string;
}) {
  const fields = (data.form?.fields ?? []).filter(
    (f) => f.source === "conversation",
  );
  const asDef = (f: PublicRequestFormField) => ({
    key: f.key,
    type: f.type,
    options: f.options,
    config: { placeholder: f.placeholder },
    label: f.label,
  });
  const [values, setValues] = useState<Record<string, unknown>>(() =>
    Object.fromEntries(
      fields.map((f) => [f.key, toInputValue(asDef(f), data.values[f.key])]),
    ),
  );
  const [dirty, setDirty] = useState(false);
  const save = useMutation({
    mutationFn: () =>
      requestFormsService.updateConversation(
        conversationUuid,
        Object.fromEntries(
          fields.map((f) => [f.key, toApiValue(asDef(f), values[f.key])]),
        ),
      ),
    onSuccess: (next) => {
      setDirty(false);
      onSaved(next);
      appToast.success(t("request_forms.toast.saved"));
    },
    onError: (error: unknown) =>
      appToast.error(
        isApiError(error) ? error.message : t("request_forms.toast.failed"),
      ),
  });
  // Conditions refer to other form fields (system and contact fields are
  // not editable here; their submitted values are not shown).
  const byKey = new Map(
    fields.map((f) => [
      f.key,
      { key: f.key, type: f.type, condition: f.condition, archived: false },
    ]),
  );
  const current = { ...data.values, ...values };
  const visible = fields.filter((f) =>
    conditionHolds(f.condition, byKey, current),
  );
  if (visible.length === 0) return null;
  return (
    <section>
      <h3 className="text-muted-foreground mb-2 text-[11px] font-semibold tracking-wide uppercase">
        {data.form?.name}
      </h3>
      <div className="space-y-2">
        {visible.map((f) => {
          const missing = data.missing_to_resolve.includes(f.key);
          return (
            <div key={f.key} className="space-y-1">
              <label
                htmlFor={`rf-${f.key}`}
                className={cn(
                  "text-xs",
                  missing ? "text-amber-700 dark:text-amber-400" : "",
                )}
              >
                {f.label}
                {data.required_to_resolve.includes(f.key) &&
                  ` · ${t("request_forms.required_to_resolve_short")}`}
              </label>
              <CustomFieldInput
                id={`rf-${f.key}`}
                definition={asDef(f) as never}
                value={values[f.key]}
                disabled={!canEdit}
                invalid={missing}
                onChange={(v) => {
                  setValues((s) => ({ ...s, [f.key]: v }));
                  setDirty(true);
                }}
              />
            </div>
          );
        })}
        {canEdit && dirty && (
          <Button
            size="sm"
            className="w-full"
            disabled={save.isPending}
            onClick={() => save.mutate()}
          >
            {t("common.save")}
          </Button>
        )}
      </div>
    </section>
  );
}
