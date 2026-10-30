/** Translation keys of the seeded tenant template roles. */
const TEMPLATE_ROLE_KEYS: Record<string, string> = {
  owner: "tenant.roles.owner",
  admin: "tenant.roles.admin",
  member: "tenant.roles.member",
  supervisor: "tenant.roles.supervisor",
  agent: "tenant.roles.agent",
  light_agent: "tenant.roles.light_agent",
  billing: "tenant.roles.billing",
  viewer: "tenant.roles.viewer",
};

/**
 * Display name of a tenant role: translated for template roles (only when the
 * role is a template; an organization role may reuse no template key, but the
 * API marks it with `is_template: false` anyway).
 */
export function tenantRoleLabel(
  t: (key: string) => string,
  role:
    { key: string; name?: string; is_template?: boolean } | null | undefined,
): string {
  if (!role) return "";
  const key =
    role.is_template === false ? undefined : TEMPLATE_ROLE_KEYS[role.key];
  if (key) return t(key);
  return role.name || role.key;
}
