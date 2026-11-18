"use client";

import { Pencil, Plus, Trash2 } from "lucide-react";
import { useMemo, useState } from "react";

import { Can } from "@/components/common/can";
import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { EntityDeleteDialog, EntityPage } from "@/components/entity";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Switch } from "@/components/ui/switch";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useBrands } from "@/features/brands/hooks/use-brands";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

import {
  useClosingTextMutations,
  useClosingTexts,
  useClosingTextScope,
} from "../hooks/use-closing-texts";
import type { ClosingText } from "../services/closing-texts.service";
import { ClosingTextFormDialog } from "./closing-text-form-dialog";

type FormState = { item?: ClosingText } | null;

/**
 * Closing texts (CF-250): the organization-wide and brand texts an agent
 * picks in the composer, the default per organization/brand, and whether
 * autonomous AI replies get the default too.
 */
export function ClosingTextsPage() {
  const { t } = useLocale();
  const { canWrite } = useClosingTextScope();
  const list = useClosingTexts();
  const brands = useBrands();
  const m = useClosingTextMutations();
  const [form, setForm] = useState<FormState>(null);
  const [deleteTarget, setDeleteTarget] = useState<ClosingText | null>(null);
  const brandName = useMemo(() => {
    const map = new Map<string, string>();
    for (const b of brands.data ?? []) map.set(b.uuid, b.name);
    return (uuid?: string | null) =>
      uuid ? (map.get(uuid) ?? "—") : t("closing_texts.organization_wide");
  }, [brands.data, t]);
  const items = list.data?.items ?? [];

  return (
    <EntityPage
      title={t("closing_texts.title")}
      description={t("closing_texts.description")}
      permission={permissions.tenant.conversations.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("closing_texts.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.cms.home },
        { label: t("closing_texts.title") },
      ]}
      actions={
        <Can permission={permissions.tenant.macros.write}>
          <Button onClick={() => setForm({})}>
            <Plus />
            {t("closing_texts.actions.create")}
          </Button>
        </Can>
      }
    >
      <div className="space-y-4">
        <label className="flex items-start gap-3 rounded-lg border p-3 text-sm">
          <Switch
            checked={list.data?.settings.apply_to_ai ?? false}
            disabled={!canWrite || !list.data || m.settings.isPending}
            onCheckedChange={(apply_to_ai) =>
              m.settings.mutate(
                { apply_to_ai },
                {
                  onSuccess: () =>
                    appToast.success(t("closing_texts.toast.settings_saved")),
                },
              )
            }
          />
          <span>
            <span className="font-medium">
              {t("closing_texts.settings.apply_to_ai")}
            </span>
            <span className="text-muted-foreground block">
              {t("closing_texts.settings.apply_to_ai_help")}
            </span>
          </span>
        </label>

        {list.isError ? (
          <ErrorState
            title={t("closing_texts.load_failed")}
            onRetry={() => void list.refetch()}
          />
        ) : !list.isLoading && items.length === 0 ? (
          <EmptyState
            title={t("closing_texts.empty_title")}
            description={t("closing_texts.empty_description")}
          />
        ) : (
          <ul className="divide-y rounded-lg border">
            {items.map((item) => (
              <li key={item.uuid} className="flex items-start gap-3 p-3">
                <div className="min-w-0 flex-1 space-y-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="font-medium">{item.name}</span>
                    <Badge variant="outline">
                      {brandName(item.brand_uuid)}
                    </Badge>
                    {item.is_default ? (
                      <Badge>{t("closing_texts.default")}</Badge>
                    ) : null}
                  </div>
                  <p className="text-muted-foreground text-sm whitespace-pre-line">
                    {item.body_tr}
                  </p>
                  {item.body_en ? (
                    <p className="text-muted-foreground text-sm whitespace-pre-line">
                      <span className="font-mono text-xs">EN </span>
                      {item.body_en}
                    </p>
                  ) : (
                    <p className="text-muted-foreground text-xs">
                      {t("closing_texts.english_missing")}
                    </p>
                  )}
                </div>
                {canWrite ? (
                  <div className="flex shrink-0 items-center gap-1">
                    <Button
                      size="icon-sm"
                      variant="ghost"
                      aria-label={t("closing_texts.actions.edit")}
                      onClick={() => setForm({ item })}
                    >
                      <Pencil />
                    </Button>
                    <Button
                      size="icon-sm"
                      variant="ghost"
                      aria-label={t("closing_texts.actions.delete")}
                      onClick={() => setDeleteTarget(item)}
                    >
                      <Trash2 />
                    </Button>
                  </div>
                ) : null}
              </li>
            ))}
          </ul>
        )}

        {list.data?.variables.length ? (
          <div className="text-muted-foreground text-xs">
            <span className="font-medium">{t("closing_texts.variables")}:</span>{" "}
            {list.data.variables.map((v) => (
              <code key={v} className="bg-muted mr-1 rounded px-1">
                {`{{${v}}}`}
              </code>
            ))}
          </div>
        ) : null}
      </div>

      <ClosingTextFormDialog
        open={form !== null}
        onOpenChange={(open) => !open && setForm(null)}
        item={form?.item}
        brands={brands.data ?? []}
      />
      <EntityDeleteDialog
        open={Boolean(deleteTarget)}
        softDelete={false}
        entityLabel={deleteTarget?.name ?? ""}
        isPending={m.remove.isPending}
        onCancel={() => setDeleteTarget(null)}
        onConfirm={() => {
          if (!deleteTarget) return;
          m.remove.mutate(deleteTarget.uuid, {
            onSuccess: () => appToast.success(t("closing_texts.toast.deleted")),
            onSettled: () => setDeleteTarget(null),
          });
        }}
      />
    </EntityPage>
  );
}
