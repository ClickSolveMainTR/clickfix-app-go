"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { conversationStatusesService } from "@/features/conversation-statuses/services/conversation-statuses.service";
import type { CustomStatusInput } from "@/features/conversation-statuses/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/** Query keys carry the organization slug. */
export const conversationStatusesKeys = {
  all: (slug: string) => ["tenant", slug, "conversation-statuses"] as const,
};

export function useConversationStatuses() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: conversationStatusesKeys.all(slug),
    queryFn: () => conversationStatusesService.list(slug),
    enabled: Boolean(slug),
  });
}

function useRefresh() {
  const { slug } = useTenant();
  const queryClient = useQueryClient();
  return () => {
    void queryClient.invalidateQueries({
      queryKey: conversationStatusesKeys.all(slug),
    });
    // The inbox header picker keeps its own copy.
    void queryClient.invalidateQueries({ queryKey: ["inbox"] });
  };
}

export function useSaveConversationStatus() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const refresh = useRefresh();
  return useMutation({
    mutationFn: ({ uuid, body }: { uuid?: string; body: CustomStatusInput }) =>
      uuid
        ? conversationStatusesService.update(slug, uuid, body)
        : conversationStatusesService.create(slug, body),
    onSuccess: (_, { uuid }) => {
      appToast.success(
        uuid
          ? t("conversation_statuses.toast.updated")
          : t("conversation_statuses.toast.created"),
      );
      refresh();
    },
  });
}

export function useDeleteConversationStatus() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const refresh = useRefresh();
  return useMutation({
    mutationFn: (uuid: string) =>
      conversationStatusesService.remove(slug, uuid),
    onSuccess: () => {
      appToast.success(t("conversation_statuses.toast.deleted"));
      refresh();
    },
  });
}
