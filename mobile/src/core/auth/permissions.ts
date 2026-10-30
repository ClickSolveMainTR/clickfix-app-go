import type { MeOrganization } from './api';
import { useSession } from './session';

/**
 * Tenant permission slugs the app checks (`tenant.*`, granted per
 * organization). Mirror of the panel's `frontend/src/config/permissions.ts`;
 * the API enforces every one of them, the app only hides what would fail.
 */
export const P = {
  conversationsRead: 'tenant.conversations.read',
  conversationsReadAll: 'tenant.conversations.read_all',
  conversationsReply: 'tenant.conversations.reply',
  conversationsNote: 'tenant.conversations.note',
  conversationsAssign: 'tenant.conversations.assign',
  conversationsManage: 'tenant.conversations.manage',
  contactsRead: 'tenant.contacts.read',
  attachmentsWrite: 'tenant.attachments.write',
  macrosRead: 'tenant.macros.read',
  teamsRead: 'tenant.teams.read',
  appointmentsRead: 'tenant.appointments.read',
  appointmentsWrite: 'tenant.appointments.write',
  videoUse: 'tenant.video.use',
  onboardingManage: 'tenant.onboarding.manage',
  contactsWrite: 'tenant.contacts.write',
  /** Knowledge base read / edit and the AI policy sit behind the settings slugs (backend `knowledge/routes.go`). */
  settingsRead: 'tenant.settings.read',
  settingsWrite: 'tenant.settings.write',
  teamChat: 'tenant.team_chat.use',
  schedulesRead: 'tenant.schedules.read',
  /** Engineering issues (`engineering/routes.go`). */
  ticketsRead: 'tenant.tickets.read',
  ticketsWrite: 'tenant.tickets.write',
  channelsRead: 'tenant.channels.read',
  channelsManage: 'tenant.channels.manage',
  membersRead: 'tenant.members.read',
  membersWrite: 'tenant.members.write',
  billingRead: 'tenant.billing.read',
} as const;

export type Permission = (typeof P)[keyof typeof P];

export function can(org: MeOrganization | null | undefined, ...anyOf: Permission[]) {
  if (!org) return false;
  return anyOf.some((p) => org.permissions.includes(p));
}

/** `useCan()(P.x, P.y)`: true when the active organization grants any of them. */
export function useCan() {
  const org = useSession((s) => s.organization);
  return (...anyOf: Permission[]) => can(org, ...anyOf);
}
