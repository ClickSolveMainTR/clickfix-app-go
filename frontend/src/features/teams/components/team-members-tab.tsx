"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { UserMinus, UserPlus } from "lucide-react";
import { useMemo, useState } from "react";

import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import { EntitySectionCard } from "@/components/entity";
import { createColumn, DataTable } from "@/components/tables";
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

  const columns = useMemo(
    () =>
      [
        createColumn<TeamMember>({
          id: "name",
          accessorFn: (row) => `${personName(row.user)} ${row.user.email}`,
          labelKey: "teams.members.columns.member",
          gridPrimary: true,
          cell: ({ row }) => (
            <div className="min-w-0">
              <p className="truncate text-sm font-medium">
                {personName(row.original.user)}
              </p>
              <p className="text-muted-foreground truncate text-xs font-normal">
                {row.original.user.email} ·{" "}
                {t("teams.members.joined", {
                  date: date(row.original.joined_at, undefined, locale),
                })}
              </p>
            </div>
          ),
        }),
        createColumn<TeamMember>({
          id: "skills",
          accessorFn: (row) => row.skills.join(" "),
          labelKey: "teams.members.skills",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.skills.length ? (
              <div className="flex flex-wrap gap-1">
                {row.original.skills.map((skill) => (
                  <Badge key={skill} variant="secondary">
                    {skill}
                  </Badge>
                ))}
              </div>
            ) : (
              <span className="text-muted-foreground">—</span>
            ),
        }),
        createColumn<TeamMember>({
          accessorKey: "role",
          labelKey: "teams.members.columns.role",
          searchable: false,
          cell: ({ row }) => (
            <label className="flex items-center gap-2 text-sm">
              <Switch
                checked={row.original.role === "lead"}
                disabled={!canWrite || busy}
                aria-label={t("teams.members.lead_hint")}
                onCheckedChange={(checked) =>
                  setMember.mutate({
                    userUuid: row.original.user.uuid,
                    role: checked ? "lead" : "member",
                  })
                }
              />
              {row.original.role === "lead"
                ? t("teams.roles.lead")
                : t("teams.roles.member")}
            </label>
          ),
        }),
        ...(canWrite
          ? [
              createColumn<TeamMember>({
                id: "actions",
                labelKey: "common.actions",
                enableSorting: false,
                meta: { headerClassName: "text-right" },
                cell: ({ row }) => (
                  <div className="flex justify-end">
                    <Button
                      type="button"
                      size="sm"
                      variant="ghost"
                      className="text-destructive"
                      disabled={busy}
                      onClick={() => setRemoving(row.original)}
                    >
                      <UserMinus className="size-4" />
                      {t("teams.members.remove")}
                    </Button>
                  </div>
                ),
              }),
            ]
          : []),
      ] as ColumnDef<TeamMember, unknown>[],
    [busy, canWrite, locale, setMember, t],
  );
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

        <DataTable
          columns={columns}
          data={members}
          getRowId={(row) => row.user.uuid}
          emptyTitle={t("teams.members.empty_title")}
          emptyDescription={t("teams.members.empty_description")}
          initialState={{ pagination: { pageIndex: 0, pageSize: 20 } }}
          features={{
            // The members come with the team: search, sort and paging run
            // in the browser.
            columnFilters: false,
            columnVisibility: false,
            density: false,
            rowSelection: false,
            viewMode: false,
          }}
        />
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
