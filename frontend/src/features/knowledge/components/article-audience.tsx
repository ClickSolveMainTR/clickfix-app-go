"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import {
  CatalogMatchEditor,
  type CustomFieldMatch,
} from "@/features/custom-fields";
import { useActiveOrganization } from "@/features/organizations";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

import {
  knowledgeAudienceService,
  knowledgeKeys,
} from "../services/knowledge.service";

const ENTITIES = ["contact", "company"] as const;

/**
 * Article targeting (CF-196): which widget visitors are offered the
 * article, picked from the custom field catalog by option id. Applies at
 * once on save, independent of the article's draft/publish cycle.
 */
export function ArticleAudience({
  articleUuid,
  readOnly,
}: {
  articleUuid: string;
  readOnly?: boolean;
}) {
  const { t } = useLocale();
  const { active: org } = useActiveOrganization();
  const qc = useQueryClient();
  const key = knowledgeKeys.audience(org?.slug, articleUuid);
  const audience = useQuery({
    queryKey: key,
    queryFn: () => knowledgeAudienceService.get(articleUuid),
    enabled: Boolean(org),
  });
  // Local edits until saved; otherwise the stored audience.
  const [draft, setDraft] = useState<CustomFieldMatch[] | null>(null);
  const value = draft ?? audience.data?.conditions ?? [];
  const save = useMutation({
    mutationFn: () =>
      knowledgeAudienceService.set(articleUuid, {
        conditions: value.filter((m) => m.key && m.options.length),
      }),
    onSuccess: (saved) => {
      qc.setQueryData(key, saved);
      setDraft(null);
      appToast.success(t("knowledge.audience.saved"));
    },
    onError: (error) =>
      appToast.error(
        isApiError(error) ? error.message : t("knowledge.audience.failed"),
      ),
  });
  const dirty =
    JSON.stringify(value) !== JSON.stringify(audience.data?.conditions ?? []);

  return (
    <div className="space-y-2 border-t pt-3">
      <Label>{t("knowledge.audience.title")}</Label>
      <p className="text-muted-foreground text-xs">
        {value.length
          ? t("knowledge.audience.targeted")
          : t("knowledge.audience.everyone")}
      </p>
      <CatalogMatchEditor
        value={value}
        onChange={setDraft}
        entities={ENTITIES}
        disabled={readOnly || audience.isLoading}
      />
      {!readOnly ? (
        <Button
          type="button"
          size="sm"
          variant="outline"
          className="w-full"
          disabled={!dirty || save.isPending}
          onClick={() => save.mutate()}
        >
          {t("knowledge.audience.save")}
        </Button>
      ) : null}
    </div>
  );
}
