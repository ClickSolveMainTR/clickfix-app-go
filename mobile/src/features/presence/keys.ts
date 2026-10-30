// Query keys only, so widgets/sync can read presence without importing presence.ts (cycle).
export const presenceKeys = { me: (org: string) => ['presence', org, 'me'] as const };
