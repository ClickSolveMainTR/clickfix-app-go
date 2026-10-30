"use client";

import { useMutation } from "@tanstack/react-query";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import { Permission } from "@/config/permissions";
import { useActiveOrganization } from "@/features/organizations";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

import { useBrands } from "../hooks/use-brands";
import { brandsService } from "../services/brands.service";
import { BrandSelect } from "./brand-select";

/**
 * Puts a knowledge article on a brand's help center (CF-124). The article
 * API does not return the current brand, so this is an explicit action
 * rather than a bound field. Needs tenant.knowledge.write like the backend.
 */
export function ArticleBrandAssign({ articleUuid }: { articleUuid: string }) {
  const { t } = useLocale();
  const { active: org } = useActiveOrganization();
  const canWrite = (org?.permissions ?? []).includes(
    Permission.TenantKnowledgeWrite,
  );
  const brands = useBrands();
  const [brandId, setBrandId] = useState<string | null>(null);
  const assign = useMutation({
    mutationFn: (brand: string) =>
      brandsService.assignArticle(brand, articleUuid),
    onSuccess: () => appToast.success(t("brands.toast.article_assigned")),
    onError: (error) =>
      appToast.error(
        isApiError(error) ? error.message : t("brands.toast.failed"),
      ),
  });
  if (!canWrite || (brands.data ?? []).length === 0) return null;
  return (
    <div className="space-y-2 border-t pt-3">
      <BrandSelect
        id="kb-brand"
        value={brandId}
        allowNone={false}
        onChange={setBrandId}
        label={t("brands.article.label")}
        hint={t("brands.article.hint")}
      />
      <Button
        type="button"
        size="sm"
        variant="outline"
        className="w-full"
        disabled={!brandId || assign.isPending}
        onClick={() => brandId && assign.mutate(brandId)}
      >
        {t("brands.article.assign")}
      </Button>
    </div>
  );
}
