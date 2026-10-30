"use client";

import { useQuery } from "@tanstack/react-query";
import { useMemo } from "react";

import { Permission } from "@/config/permissions";
import { teamsService } from "@/features/teams/services/teams.service";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantMembersService } from "@/features/tenant-members/services/tenant-members.service";

export type DirectoryOption = { value: string; label: string };

/**
 * First page (100) of members and teams of the active organization as
 * picker options plus uuid → name lookups. Reports, rotas and escalation
 * steps reference people and teams by uuid only. Without
 * `tenant.members.read` / `tenant.teams.read` the lists stay empty and
 * callers fall back to a short uuid.
 */
export function useMemberDirectory() {
  const { slug, permissions } = useTenant();
  const canMembers = permissions.includes(Permission.TenantMembersRead);
  const canTeams = permissions.includes(Permission.TenantTeamsRead);
  const members = useQuery({
    queryKey: ["tenant", slug, "members", "directory"],
    queryFn: () =>
      tenantMembersService.list(slug, { limit: 100, offset: 0, sort: "name" }),
    enabled: Boolean(slug) && canMembers,
    staleTime: 60_000,
  });
  const teams = useQuery({
    queryKey: ["tenant", slug, "teams", "directory"],
    queryFn: () =>
      teamsService.list(slug, { limit: 100, offset: 0, sort: "name" }),
    enabled: Boolean(slug) && canTeams,
    staleTime: 60_000,
  });

  return useMemo(() => {
    const memberOptions: DirectoryOption[] = (members.data?.items ?? []).map(
      (m) => ({
        value: m.user.uuid,
        label:
          [m.user.name, m.user.surname].filter(Boolean).join(" ").trim() ||
          m.user.email,
      }),
    );
    const teamOptions: DirectoryOption[] = (teams.data?.items ?? []).map(
      (team) => ({ value: team.uuid, label: team.name }),
    );
    const memberNames = new Map(memberOptions.map((o) => [o.value, o.label]));
    const teamNames = new Map(teamOptions.map((o) => [o.value, o.label]));
    return {
      members: memberOptions,
      teams: teamOptions,
      memberName: (uuid?: string | null) =>
        uuid ? (memberNames.get(uuid) ?? shortId(uuid)) : "—",
      teamName: (uuid?: string | null) =>
        uuid ? (teamNames.get(uuid) ?? shortId(uuid)) : "—",
    };
  }, [members.data, teams.data]);
}

export function shortId(uuid: string) {
  return uuid.slice(0, 8);
}
