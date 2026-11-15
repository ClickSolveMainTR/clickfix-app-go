"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { categoriesService } from "@/features/categories/services/categories.service";
import type {
  CategoryInput,
  CategorySubjectType,
} from "@/features/categories/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/** Query keys carry the organization slug. */
export const categoriesKeys = {
  all: (slug: string) => ["tenant", slug, "categories"] as const,
  list: (slug: string, includeArchived: boolean) =>
    [...categoriesKeys.all(slug), "list", includeArchived] as const,
  review: (slug: string, offset: number) =>
    [...categoriesKeys.all(slug), "review", offset] as const,
};

export function useCategories(includeArchived: boolean) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: categoriesKeys.list(slug, includeArchived),
    queryFn: () => categoriesService.list(slug, includeArchived),
    enabled: Boolean(slug),
  });
}

export function useCategoryReview(offset: number, enabled = true) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: categoriesKeys.review(slug, offset),
    queryFn: () => categoriesService.review(slug, { limit: 20, offset }),
    placeholderData: (previous) => previous,
    enabled: enabled && Boolean(slug),
  });
}

function useRefresh() {
  const { slug } = useTenant();
  const queryClient = useQueryClient();
  return () =>
    void queryClient.invalidateQueries({ queryKey: categoriesKeys.all(slug) });
}

export function useSaveCategory() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const refresh = useRefresh();
  return useMutation({
    mutationFn: ({ uuid, body }: { uuid?: string; body: CategoryInput }) =>
      uuid
        ? categoriesService.update(slug, uuid, body)
        : categoriesService.create(slug, body),
    onSuccess: (_, { uuid }) => {
      appToast.success(
        uuid ? t("categories.toast.updated") : t("categories.toast.created"),
      );
      refresh();
    },
  });
}

export function useDeleteCategory() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (uuid: string) => categoriesService.remove(slug, uuid),
    onSuccess: () => {
      appToast.success(t("categories.toast.deleted"));
      refresh();
    },
  });
}

export function useLabelRecord() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (v: {
      subjectType: CategorySubjectType;
      subjectUuid: string;
      categoryUuid: string;
    }) =>
      categoriesService.label(
        slug,
        v.subjectType,
        v.subjectUuid,
        v.categoryUuid,
      ),
    onSuccess: () => {
      appToast.success(t("categories.toast.labeled"));
      refresh();
    },
  });
}

export function useExportLabels() {
  const { slug } = useTenant();
  return useMutation({
    mutationFn: async () => {
      const { blob, filename } = await categoriesService.exportLabels(slug);
      const href = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = href;
      a.download = filename ?? "category-labels.jsonl";
      a.click();
      URL.revokeObjectURL(href);
    },
  });
}
