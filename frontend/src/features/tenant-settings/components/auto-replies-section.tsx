"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useMemo } from "react";
import { z } from "zod";

import { Loading } from "@/components/common/loading";
import {
  AppForm,
  AppSwitch,
  AppTextarea,
  FormActions,
  FormSection,
} from "@/components/forms";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import {
  autoRepliesService,
  type AutoReplySettings,
} from "@/features/tenant-settings/services/auto-replies.service";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

const KINDS = ["ack", "offline", "handoff"] as const;
const LOCALES = ["tr", "en"] as const;

type Values = {
  ack: { enabled: boolean; tr: string; en: string };
  offline: { enabled: boolean; tr: string; en: string };
  handoff: { enabled: boolean; tr: string; en: string };
};

const reply = z.object({
  enabled: z.boolean(),
  tr: z.string().trim().max(1000),
  en: z.string().trim().max(1000),
});
const schema = z.object({ ack: reply, offline: reply, handoff: reply });

function toValues(s: AutoReplySettings): Values {
  const one = (r: AutoReplySettings["ack"]) => ({
    enabled: r.enabled,
    tr: r.text.tr ?? "",
    en: r.text.en ?? "",
  });
  return { ack: one(s.ack), offline: one(s.offline), handoff: one(s.handoff) };
}

function toBody(v: Values): AutoReplySettings {
  const one = (r: Values["ack"]) => ({
    enabled: r.enabled,
    text: { tr: r.tr, en: r.en },
  });
  return {
    ack: one(v.ack),
    offline: one(v.offline),
    handoff: one(v.handoff),
  };
}

/**
 * Otomatik yanıtlar (CF-198): the acknowledgement every new conversation
 * gets inside working hours, the out-of-hours message and the handoff notice
 * the AI agent sends when a person takes over. One source for each text;
 * the hours come from the SLA schedule.
 */
export function AutoRepliesSection({ disabled }: { disabled?: boolean }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const queryClient = useQueryClient();
  const key = useMemo(
    () => ["tenant", slug, "organization", "auto-replies"] as const,
    [slug],
  );
  const query = useQuery({
    queryKey: key,
    queryFn: () => autoRepliesService.get(slug),
  });
  const save = useMutation({
    mutationFn: (body: AutoReplySettings) =>
      autoRepliesService.update(slug, body),
    onSuccess: (data) => {
      queryClient.setQueryData(key, data);
      appToast.success(t("tenant_settings.auto_replies.saved"));
    },
  });
  const defaults = useMemo(
    () => (query.data ? toValues(query.data) : undefined),
    [query.data],
  );

  if (query.isLoading || !defaults) {
    return (
      <FormSection
        title={t("tenant_settings.auto_replies.title")}
        description={t("tenant_settings.auto_replies.description")}
      >
        {query.isError ? (
          <Button variant="outline" onClick={() => void query.refetch()}>
            {t("common.retry")}
          </Button>
        ) : (
          <>
            <Loading label={t("common.loading")} />
            <Skeleton className="h-24 w-full" />
          </>
        )}
      </FormSection>
    );
  }

  return (
    <AppForm<Values>
      schema={schema}
      defaultValues={defaults}
      onSubmit={async (values) => {
        await save.mutateAsync(toBody(values));
      }}
    >
      {(form) => (
        <FormSection
          title={t("tenant_settings.auto_replies.title")}
          description={t("tenant_settings.auto_replies.description")}
        >
          {KINDS.map((kind) => (
            <div key={kind} className="space-y-3">
              <AppSwitch
                name={`${kind}.enabled`}
                label={t(`tenant_settings.auto_replies.${kind}`)}
                description={t(`tenant_settings.auto_replies.${kind}_hint`)}
                disabled={disabled}
              />
              <div className="grid gap-3 sm:grid-cols-2">
                {LOCALES.map((loc) => (
                  <AppTextarea
                    key={loc}
                    name={`${kind}.${loc}`}
                    label={t(`tenant_settings.auto_replies.text_${loc}`)}
                    disabled={disabled}
                    rows={3}
                  />
                ))}
              </div>
            </div>
          ))}
          <p className="text-muted-foreground text-xs">
            {t("tenant_settings.auto_replies.eta_hint")}
          </p>
          {disabled ? null : (
            <FormActions>
              <Button
                type="submit"
                disabled={
                  !form.formState.isDirty || form.formState.isSubmitting
                }
              >
                {t("tenant_settings.save_bar.save")}
              </Button>
            </FormActions>
          )}
        </FormSection>
      )}
    </AppForm>
  );
}
