"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { permissions } from "@/config/permissions";
import { departmentsService } from "@/features/departments/services/departments.service";
import type {
  Department,
  DepartmentInput,
  DepartmentShareInput,
  DepartmentTransferInput,
  ListDepartmentsParams,
} from "@/features/departments/types";
import { teamsKeys } from "@/features/teams/hooks/use-teams";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

/** Query keys carry the organization slug. */
export const departmentsKeys = {
  all: (slug: string) => ["tenant", slug, "departments"] as const,
  list: (slug: string, params: Record<string, unknown>) =>
    [...departmentsKeys.all(slug), "list", params] as const,
  detail: (slug: string, uuid: string) =>
    [...departmentsKeys.all(slug), "detail", uuid] as const,
  shares: (slug: string, uuid: string) =>
    [...departmentsKeys.all(slug), "shares", uuid] as const,
  conversation: (slug: string, uuid: string) =>
    ["tenant", slug, "conversation-department", uuid] as const,
};

export function useDepartmentsList(
  params: ListDepartmentsParams,
  enabled = true,
) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: departmentsKeys.list(slug, params),
    queryFn: () => departmentsService.list(slug, params),
    placeholderData: (previous) => previous,
    enabled: enabled && Boolean(slug),
  });
}

/** Every department as select options (the list is small, one page). */
export function useDepartmentOptions() {
  const { can } = usePermission();
  const query = useDepartmentsList(
    { limit: 100, offset: 0 },
    can(permissions.tenant.departments.read),
  );
  return {
    ...query,
    options: (query.data?.items ?? []).map((d) => ({
      value: d.uuid,
      label: d.name,
    })),
  };
}

export function useDepartment(uuid: string) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: departmentsKeys.detail(slug, uuid),
    queryFn: () => departmentsService.get(slug, uuid),
    enabled: Boolean(slug && uuid),
  });
}

function useDepartmentRefresh() {
  const { slug } = useTenant();
  const queryClient = useQueryClient();
  return (department?: Department) => {
    if (department)
      queryClient.setQueryData(
        departmentsKeys.detail(slug, department.uuid),
        department,
      );
    void queryClient.invalidateQueries({
      queryKey: [...departmentsKeys.all(slug), "list"],
    });
    // Team lists show the department of each team.
    void queryClient.invalidateQueries({ queryKey: teamsKeys.all(slug) });
  };
}

export function useCreateDepartment() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const refresh = useDepartmentRefresh();
  return useMutation({
    mutationFn: (body: DepartmentInput) =>
      departmentsService.create(slug, body),
    onSuccess: (department) => {
      appToast.success(t("departments.toast.created"));
      refresh(department);
    },
  });
}

export function useUpdateDepartment(uuid: string) {
  const { slug } = useTenant();
  const { t } = useLocale();
  const refresh = useDepartmentRefresh();
  return useMutation({
    mutationFn: (body: DepartmentInput) =>
      departmentsService.update(slug, uuid, body),
    onSuccess: (department) => {
      appToast.success(t("departments.toast.updated"));
      refresh(department);
    },
  });
}

export function useDeleteDepartment() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const refresh = useDepartmentRefresh();
  return useMutation({
    mutationFn: ({ uuid, moveTo }: { uuid: string; moveTo?: string }) =>
      departmentsService.remove(slug, uuid, moveTo),
    onSuccess: () => {
      appToast.success(t("departments.toast.deleted"));
      refresh();
    },
  });
}

export function useDepartmentShares(uuid: string) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: departmentsKeys.shares(slug, uuid),
    queryFn: () => departmentsService.shares(slug, uuid),
    enabled: Boolean(slug && uuid),
  });
}

export function useShareKnowledge() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({
      target,
      body,
    }: {
      target: string;
      body: DepartmentShareInput;
    }) => departmentsService.share(slug, target, body),
    onSuccess: () => {
      appToast.success(t("departments.toast.shared"));
      void queryClient.invalidateQueries({
        queryKey: [...departmentsKeys.all(slug), "shares"],
      });
    },
  });
}

export function useUnshareKnowledge() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({
      department,
      share,
    }: {
      department: string;
      share: string;
    }) => departmentsService.unshare(slug, department, share),
    onSuccess: () => {
      appToast.success(t("departments.toast.unshared"));
      void queryClient.invalidateQueries({
        queryKey: [...departmentsKeys.all(slug), "shares"],
      });
    },
  });
}

export function useConversationDepartment(
  conversation: string,
  enabled = true,
) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: departmentsKeys.conversation(slug, conversation),
    queryFn: () => departmentsService.conversation(slug, conversation),
    enabled: enabled && Boolean(slug && conversation),
  });
}

export function useTransferConversation(conversation: string) {
  const { slug } = useTenant();
  const { t } = useLocale();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (body: DepartmentTransferInput) =>
      departmentsService.transfer(slug, conversation, body),
    onSuccess: (state) => {
      appToast.success(t("departments.toast.transferred"));
      queryClient.setQueryData(
        departmentsKeys.conversation(slug, conversation),
        state,
      );
      // The team may have been released: refresh the conversation.
      void queryClient.invalidateQueries({
        queryKey: ["inbox", slug],
      });
    },
  });
}
