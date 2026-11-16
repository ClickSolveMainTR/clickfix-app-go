import { Permission } from "@/config/permissions";
import { useActiveOrganization } from "@/features/organizations";

/** Active organization slug and the remote support permissions it grants. */
export function useRemoteSupportScope() {
  const { active } = useActiveOrganization();
  const perms = active?.permissions ?? [];
  return {
    slug: active?.slug ?? "",
    use: perms.includes(Permission.TenantRemoteSupportUse),
    control: perms.includes(Permission.TenantRemoteSupportControl),
    settings: perms.includes(Permission.TenantRemoteSupportSettings),
  };
}

export const remoteSupportKeys = {
  all: (slug: string) => ["remote-support", slug] as const,
  conversation: (slug: string, conversation: string) =>
    ["remote-support", slug, "conversation", conversation] as const,
  session: (slug: string, uuid: string) =>
    ["remote-support", slug, "session", uuid] as const,
  settings: (slug: string) => ["remote-support", slug, "settings"] as const,
};
