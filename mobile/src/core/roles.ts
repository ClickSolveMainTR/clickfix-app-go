import { i18n } from './i18n';

type Role = { key?: string; name: string; description?: string; is_template?: boolean };
const TEMPLATE_ROLES: Record<string, string> = {
  owner: 'owner',
  admin: 'admin',
  member: 'member',
  supervisor: 'supervisor',
  agent: 'agent',
  org_owner: 'owner',
  org_admin: 'admin',
  org_member: 'member',
  'Organization Owner': 'owner',
  'Organization Admin': 'admin',
  'Organization Member': 'member',
  Supervisor: 'supervisor',
  Agent: 'agent',
};

// Match the panel: explicitly custom roles always keep their own text.
function templateKey(role: Role) {
  return role.is_template === false ? undefined : TEMPLATE_ROLES[role.key ?? role.name];
}
export function roleLabel(role: Role) {
  const key = templateKey(role);
  return key ? i18n.t(`hub:roles.${key}`) : role.name;
}
export function roleDescription(role: Role) {
  const key = templateKey(role);
  return key ? i18n.t(`hub:roles.${key}_description`) : role.description;
}
