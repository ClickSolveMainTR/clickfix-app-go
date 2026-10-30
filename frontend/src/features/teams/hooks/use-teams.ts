"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { teamsService } from "@/features/teams/services/teams.service";
import type {
  ListTeamsParams,
  Team,
  TeamInput,
  TeamRole,
} from "@/features/teams/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/** Query keys carry the organization slug. */
export const teamsKeys = {
  all: (slug: string) => ["tenant", slug, "teams"] as const,
  list: (slug: string, params: Record<string, unknown>) =>
    [...teamsKeys.all(slug), "list", params] as const,
  detail: (slug: string, uuid: string) =>
    [...teamsKeys.all(slug), "detail", uuid] as const,
  skills: (slug: string) => ["tenant", slug, "skills"] as const,
};

export function useTeamsList(params: ListTeamsParams, enabled = true) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: teamsKeys.list(slug, params),
    queryFn: () => teamsService.list(slug, params),
    placeholderData: (previous) => previous,
    enabled: enabled && Boolean(slug),
  });
}

export function useTeam(uuid: string) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: teamsKeys.detail(slug, uuid),
    queryFn: () => teamsService.get(slug, uuid),
    enabled: Boolean(slug && uuid),
  });
}

export function useSkillCatalog(enabled = true) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: teamsKeys.skills(slug),
    queryFn: () => teamsService.skills(slug),
    enabled: enabled && Boolean(slug),
    staleTime: 60_000,
  });
}

/** Stores a returned team and refreshes lists and the skill catalog. */
function useTeamRefresh() {
  const { slug } = useTenant();
  const queryClient = useQueryClient();
  return (team?: Team) => {
    if (team) queryClient.setQueryData(teamsKeys.detail(slug, team.uuid), team);
    void queryClient.invalidateQueries({
      queryKey: [...teamsKeys.all(slug), "list"],
    });
    void queryClient.invalidateQueries({ queryKey: teamsKeys.skills(slug) });
  };
}

export function useCreateTeam() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const refresh = useTeamRefresh();
  return useMutation({
    mutationFn: (body: TeamInput) => teamsService.create(slug, body),
    onSuccess: (team) => {
      appToast.success(t("teams.toast.created"));
      refresh(team);
    },
  });
}

export function useUpdateTeam(uuid: string) {
  const { slug } = useTenant();
  const { t } = useLocale();
  const refresh = useTeamRefresh();
  return useMutation({
    mutationFn: (body: TeamInput) => teamsService.update(slug, uuid, body),
    onSuccess: (team) => {
      appToast.success(t("teams.toast.updated"));
      refresh(team);
    },
  });
}

export function useDeleteTeam() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const refresh = useTeamRefresh();
  return useMutation({
    mutationFn: (uuid: string) => teamsService.remove(slug, uuid),
    onSuccess: () => {
      appToast.success(t("teams.toast.deleted"));
      refresh();
    },
  });
}

export function useSetTeamMember(uuid: string) {
  const { slug } = useTenant();
  const { t } = useLocale();
  const refresh = useTeamRefresh();
  return useMutation({
    mutationFn: ({ userUuid, role }: { userUuid: string; role: TeamRole }) =>
      teamsService.setMember(slug, uuid, userUuid, role),
    onSuccess: (team) => {
      appToast.success(t("teams.toast.member_set"));
      refresh(team);
    },
  });
}

export function useRemoveTeamMember(uuid: string) {
  const { slug } = useTenant();
  const { t } = useLocale();
  const refresh = useTeamRefresh();
  return useMutation({
    mutationFn: (userUuid: string) =>
      teamsService.removeMember(slug, uuid, userUuid),
    onSuccess: (team) => {
      appToast.success(t("teams.toast.member_removed"));
      refresh(team);
    },
  });
}

export function useSetMemberSkills() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({
      userUuid,
      skills,
    }: {
      userUuid: string;
      skills: string[];
    }) => teamsService.setMemberSkills(slug, userUuid, skills),
    onSuccess: () => {
      appToast.success(t("teams.skills.saved"));
      void queryClient.invalidateQueries({ queryKey: teamsKeys.all(slug) });
      void queryClient.invalidateQueries({ queryKey: teamsKeys.skills(slug) });
      void queryClient.invalidateQueries({
        queryKey: ["tenant", slug, "members"],
      });
    },
  });
}
