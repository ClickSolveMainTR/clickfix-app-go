"use client";

import { UserMinus, UserPlus } from "lucide-react";
import { useMemo, useState } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import { EntitySectionCard } from "@/components/entity";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { AsyncCombobox } from "@/components/ui/async-combobox";
import { Switch } from "@/components/ui/switch";
import { permissions } from "@/config/permissions";
import {
  useRemoveTeamMember,
  useSetTeamMember,
} from "@/features/teams/hooks/use-teams";
import { personName, type Team, type TeamMember } from "@/features/teams/types";
import { useTenantMembersList } from "@/features/tenant-members/hooks/use-tenant-members";
import { date } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

const ORG_MEMBERS_PARAMS = { limit: 100, offset: 0, sort: "name" };

/**
 * "Üyeler" tab of a team: members with their team role (lead toggle) and
 * skills, add from the organization's members, remove from the team.
 */
export function TeamMembersTab({ team }: { team: Team }) {
  const { t, locale } = useLocale();
  const { can } = usePermission();
  const canWrite = can(permissions.tenant.teams.write);
  const setMember = useSetTeamMember(team.uuid);
  const removeMember = useRemoveTeamMember(team.uuid);
  const [picked, setPicked] = useState("");
  const [removing, setRemoving] = useState<TeamMember | null>(null);
  const members = useMemo(() => team.members ?? [], [team.members]);

  const orgMembers = useTenantMembersList(ORG_MEMBERS_PARAMS);
  const options = useMemo(() => {
    const inTeam = new Set(members.map((m) => m.user.uuid));
    return (orgMembers.data?.items ?? [])
      .filter((m) => !inTeam.has(m.user.uuid))
      .map((m) => ({
        value: m.user.uuid,
        label: personName(m.user),
        description: m.user.email,
      }));
  }, [members, orgMembers.data?.items]);

  const busy = setMember.isPending || removeMember.isPending;
  const removingName = removing ? personName(removing.user) : "";

  return (
    <EntitySectionCard title={t("teams.members.title")} badge={members.length}>
      <div className="space-y-4">
        {canWrite ? (
          <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
            <AsyncCombobox
              value={picked}
              onValueChange={setPicked}
              options={options}
              placeholder={t("teams.members.add_placeholder")}
              searchPlaceholder={t("teams.members.add_search")}
              emptyText={t("teams.members.add_empty")}
              className="sm:max-w-sm"
            />
            <Button
              type="button"
              size="sm"
              disabled={!picked || busy}
              onClick={() =>
                setMember.mutate(
                  { userUuid: picked, role: "member" },
                  { onSuccess: () => setPicked("") },
                )
              }
            >
              <UserPlus className="size-4" />
              {t("teams.members.add")}
            </Button>
          </div>
        ) : null}

        {members.length === 0 ? (
          <EmptyState
            title={t("teams.members.empty_title")}
            description={t("teams.members.empty_description")}
          />
        ) : (
          <ul className="divide-y rounded-lg border">
            {members.map((member) => (
              <li
                key={member.user.uuid}
                className="flex flex-col gap-3 p-3 sm:flex-row sm:items-center"
              >
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm font-medium">
                    {personName(member.user)}
                  </p>
                  <p className="text-muted-foreground truncate text-xs">
                    {member.user.email} ·{" "}
                    {t("teams.members.joined", {
                      date: date(member.joined_at, undefined, locale),
                    })}
                  </p>
                  {member.skills.length ? (
                    <div className="mt-1.5 flex flex-wrap gap-1">
                      {member.skills.map((skill) => (
                        <Badge key={skill} variant="secondary">
                          {skill}
                        </Badge>
                      ))}
                    </div>
                  ) : null}
                </div>
                <label className="flex items-center gap-2 text-sm">
                  <Switch
                    checked={member.role === "lead"}
                    disabled={!canWrite || busy}
                    aria-label={t("teams.members.lead_hint")}
                    onCheckedChange={(checked) =>
                      setMember.mutate({
                        userUuid: member.user.uuid,
                        role: checked ? "lead" : "member",
                      })
                    }
                  />
                  {member.role === "lead"
                    ? t("teams.roles.lead")
                    : t("teams.roles.member")}
                </label>
                {canWrite ? (
                  <Button
                    type="button"
                    size="sm"
                    variant="ghost"
                    className="text-destructive"
                    disabled={busy}
                    onClick={() => setRemoving(member)}
                  >
                    <UserMinus className="size-4" />
                    {t("teams.members.remove")}
                  </Button>
                ) : null}
              </li>
            ))}
          </ul>
        )}
      </div>

      <ConfirmDialog
        open={Boolean(removing)}
        title={t("teams.members.remove_title", { name: removingName })}
        description={t("teams.members.remove_description")}
        confirmLabel={t("teams.members.remove")}
        variant="destructive"
        isPending={removeMember.isPending}
        onCancel={() => setRemoving(null)}
        onConfirm={() => {
          if (!removing) return;
          removeMember.mutate(removing.user.uuid, {
            onSettled: () => setRemoving(null),
          });
        }}
      />
    </EntitySectionCard>
  );
}
