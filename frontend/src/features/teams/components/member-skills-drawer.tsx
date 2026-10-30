"use client";

import { useState } from "react";

import { EntityDrawer } from "@/components/entity";
import { Button } from "@/components/ui/button";
import { SkillsInput } from "@/features/teams/components/skills-input";
import {
  useSetMemberSkills,
  useSkillCatalog,
} from "@/features/teams/hooks/use-teams";
import { personName } from "@/features/teams/types";
import type { TenantMember } from "@/features/tenant-members/types";
import { useLocale } from "@/providers/locale-provider";

function SkillsEditor({
  member,
  onOpenChange,
}: {
  member: TenantMember;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const catalog = useSkillCatalog();
  const save = useSetMemberSkills();
  const [skills, setSkills] = useState<string[]>(member.skills);

  return (
    <div className="space-y-4 px-4">
      <SkillsInput
        value={skills}
        onChange={setSkills}
        suggestions={catalog.data?.items ?? []}
      />
      <p className="text-muted-foreground text-xs">
        {t("teams.fields.skills_hint")}
      </p>
      <div className="flex justify-end gap-2">
        <Button
          type="button"
          variant="outline"
          onClick={() => onOpenChange(false)}
        >
          {t("form.cancel")}
        </Button>
        <Button
          type="button"
          disabled={save.isPending}
          onClick={() =>
            save.mutate(
              { userUuid: member.user.uuid, skills },
              { onSuccess: () => onOpenChange(false) },
            )
          }
        >
          {t("teams.skills.save")}
        </Button>
      </div>
    </div>
  );
}

/** "Beceriler" drawer of the members page (`PUT /v1/tenant/members/{uuid}/skills`). */
export function MemberSkillsDrawer({
  member,
  onOpenChange,
}: {
  member: TenantMember | null;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  return (
    <EntityDrawer
      open={Boolean(member)}
      onOpenChange={onOpenChange}
      title={t("teams.skills.title")}
      description={
        member
          ? t("teams.skills.description", { name: personName(member.user) })
          : undefined
      }
      size="md"
    >
      {member ? (
        <SkillsEditor
          key={member.user.uuid}
          member={member}
          onOpenChange={onOpenChange}
        />
      ) : null}
    </EntityDrawer>
  );
}
