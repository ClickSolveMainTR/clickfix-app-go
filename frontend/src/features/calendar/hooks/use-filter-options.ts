"use client";

import { useQuery } from "@tanstack/react-query";

import { teamsService } from "@/features/teams";
import { tenantMembersService } from "@/features/tenant-members";
import { useTenant } from "@/features/tenant/components/tenant-provider";

type Option = { uuid: string; name: string };

/** Members and teams for the calendar host / team filters. */
export function useCalendarFilterOptions(enabled = true) {
  const { slug } = useTenant();
  const members = useQuery({
    queryKey: ["tenant", slug, "calendar", "filter-members"],
    queryFn: () => tenantMembersService.list(slug, { limit: 100, offset: 0 }),
    enabled: enabled && Boolean(slug),
    staleTime: 5 * 60_000,
    retry: false,
  });
  const teams = useQuery({
    queryKey: ["tenant", slug, "calendar", "filter-teams"],
    queryFn: () => teamsService.list(slug, { limit: 100, offset: 0 }),
    enabled: enabled && Boolean(slug),
    staleTime: 5 * 60_000,
    retry: false,
  });
  const memberOptions: Option[] = (members.data?.items ?? []).map((m) => ({
    uuid: m.user.uuid,
    name: `${m.user.name} ${m.user.surname}`.trim() || m.user.email,
  }));
  const teamOptions: Option[] = (teams.data?.items ?? []).map((team) => ({
    uuid: team.uuid,
    name: team.name,
  }));
  return { members: memberOptions, teams: teamOptions };
}
