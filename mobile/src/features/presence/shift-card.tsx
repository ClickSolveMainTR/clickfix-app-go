import { useQuery } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { View } from 'react-native';

import { api, orgHeader, unwrap } from '@/core/api/client';
import { P, useCan } from '@/core/auth/permissions';
import { useSession } from '@/core/auth/session';
import { displayTimeZone } from '@/core/format';
import { useNow } from '@/features/inbox/sla-badge';
import { Card } from '@/ui/card';
import { Text } from '@/ui/text';

import { shiftToday } from './shift-core';

/**
 * "My shift" (CF-82): today's working hours of the member's teams
 * (`/v1/tenant/teams?member=me`), in each team's time zone, and whether the
 * shift is on now. Teams on the organization's business hours say so.
 */
export function ShiftCard() {
  const { t } = useTranslation('presence');
  const can = useCan();
  const org = useSession((s) => s.organization?.slug ?? '');
  const me = useSession((s) => s.me?.user.uuid ?? '');
  const now = useNow(60_000);
  const teams = useQuery({
    queryKey: ['teams', org, 'member', me],
    enabled: !!org && !!me && can(P.teamsRead),
    staleTime: 10 * 60_000,
    queryFn: () =>
      unwrap(api.GET('/v1/tenant/teams', { params: { header: orgHeader(), query: { member: me, limit: 20 } } })),
  });
  const items = teams.data?.items ?? [];
  if (!items.length) return null;
  return (
    <Card title={t('shift')}>
      {items.map((team) => {
        const zone = team.timezone || displayTimeZone();
        const s = shiftToday(team.working_hours?.days ?? null, new Date(now), zone);
        const line =
          s.state === 'org'
            ? t('shift_org')
            : s.state === 'closed'
              ? t('shift_closed')
              : t(`shift_${s.state}`, { start: s.start, end: s.end });
        return (
          <View key={team.uuid} className="gap-0.5">
            <Text className="font-sans-semibold">{team.name}</Text>
            <Text variant="footnote" tone={s.state === 'on' ? 'signal' : 'muted'}>
              {line}
              {s.state !== 'org' && team.timezone ? ` · ${team.timezone}` : ''}
            </Text>
          </View>
        );
      })}
    </Card>
  );
}
