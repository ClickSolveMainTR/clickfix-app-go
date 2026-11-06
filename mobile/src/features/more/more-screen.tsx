import { openAssistant } from '@/features/assistant/open';
import { openFeedback } from '@/features/feedback/store';
import { useHelpAvailability } from '@/features/guides/api';
import { GuideTarget } from '@/ui/guide-target';
import { roleLabel } from '@/core/roles';
import { useCompactHeader } from '@/features/shell/compact-header';
import { type Href, router, Stack } from 'expo-router';
import * as WebBrowser from 'expo-web-browser';
import { useTranslation } from 'react-i18next';
import { View } from 'react-native';

import { P, useCan } from '@/core/auth/permissions';
import { useSession } from '@/core/auth/session';
import { WEB_URL } from '@/core/config';
import { useMyPresence } from '@/features/presence/presence';
import { useTeamChatUnread } from '@/features/team/api';
import { Avatar } from '@/ui/avatar';
import type { IconName } from '@/ui/icons';
import { List, ListRow, SectionHeader } from '@/ui/list';
import { Screen } from '@/ui/screen';
import { Text } from '@/ui/text';

type Item = { key: string; icon: IconName; to: Href; show: boolean; badge?: number };

const PRESENCE_DOT: Record<string, string> = {
  online: 'bg-success',
  away: 'bg-warning',
  busy: 'bg-destructive',
  offline: 'bg-muted-foreground',
};

/**
 * "More" hub (CF-79): who I am and where (organization switch), my status,
 * then every area that is not one of the four daily tabs, grouped the way
 * the panel's sidebar groups them. An area the member's role cannot use is
 * not listed; the API still enforces each one.
 */
export function MoreScreen() {
  const { t } = useTranslation('hub');
  const help = useHelpAvailability();
  const compactHeader = useCompactHeader(t('more.title'));
  const can = useCan();
  const me = useSession((s) => s.me);
  const org = useSession((s) => s.organization);
  const presence = useMyPresence();
  const chatUnread = useTeamChatUnread();
  const user = me?.user;
  const name = [user?.name, user?.surname].filter(Boolean).join(' ') || user?.email || '';
  const orgCount = me?.organizations?.length ?? 0;

  const groups: { title: string; items: Item[] }[] = [
    {
      title: t('more.group_find'),
      items: [
        { key: 'dashboard', icon: 'home', to: '/more/dashboard', show: true },
        { key: 'search', icon: 'search', to: '/search', show: true },
        { key: 'agenda', icon: 'calendar', to: '/agenda', show: can(P.appointmentsRead) },
        { key: 'video', icon: 'video', to: '/more/video', show: can(P.videoUse) },
      ],
    },
    {
      title: t('more.group_team'),
      items: [
        { key: 'team', icon: 'customers', to: '/more/team', show: true },
        { key: 'team_chat', icon: 'chat', to: '/more/team-chat', show: can(P.teamChat), badge: chatUnread },
        { key: 'oncall', icon: 'alert', to: '/more/oncall', show: can(P.schedulesRead) },
        { key: 'workforce', icon: 'calendar', to: '/more/workforce', show: can(P.schedulesRead) },
        { key: 'coaching', icon: 'chart', to: '/more/coaching', show: can(P.schedulesRead, P.qualityManage) },
      ],
    },
    {
      title: t('more.group_work'),
      items: [
        { key: 'reports', icon: 'chart', to: '/more/reports', show: can(P.conversationsReadAll) },
        { key: 'engineering', icon: 'bug', to: '/more/engineering', show: can(P.ticketsRead) },
        { key: 'incidents', icon: 'megaphone', to: '/more/incidents', show: can(P.conversationsRead) },
        { key: 'knowledge', icon: 'book', to: '/more/knowledge', show: can(P.settingsRead) },
        {
          key: 'objects',
          icon: 'grid',
          to: '/more/objects',
          show: can(P.customFieldsRead) && can(P.customObjectsRead),
        },
        { key: 'apps', icon: 'grid', to: '/more/apps', show: can(P.conversationsRead) },
        { key: 'replies', icon: 'note', to: '/more/replies', show: can(P.macrosRead) },
        { key: 'workflows', icon: 'split', to: '/more/workflows', show: can(P.automationsRead) },
        { key: 'rules', icon: 'macro', to: '/more/rules', show: can(P.automationsRead) },
      ],
    },
    {
      title: t('more.group_admin'),
      items: [
        { key: 'channels', icon: 'channel', to: '/more/channels', show: can(P.channelsRead) },
        { key: 'members', icon: 'staff', to: '/more/members', show: can(P.membersRead) },
        { key: 'teams', icon: 'customers', to: '/more/teams', show: can(P.teamsRead) },
        { key: 'forum', icon: 'chat', to: '/more/forum', show: can(P.feedbackManage) },
        { key: 'ai', icon: 'sparkles', to: '/more/ai', show: can(P.settingsRead) },
        { key: 'billing', icon: 'wallet', to: '/more/billing', show: can(P.billingRead) },
      ],
    },
    {
      title: t('more.group_app'),
      items: [
        { key: 'settings', icon: 'settings', to: '/more/settings', show: true },
        { key: 'qr', icon: 'scan', to: '/qr-scan', show: true },
      ],
    },
  ];

  return (
    <>
      <Stack.Screen options={{ ...compactHeader }} />
      <Screen>
        {help.assistant || help.guides || help.feedback ? <GuideTarget id="more.help">
          <SectionHeader title={t('help:more.group')} />
          <List>
            {help.assistant ? (
              <ListRow
                iconTile
                icon="sparkles"
                title={t('help:more.assistant')}
                subtitle={t('help:more.assistant_hint')}
                last={!help.guides && !help.feedback}
                onPress={() => openAssistant({ route: '/more' })}
              />
            ) : null}
            {help.guides ? (
              <ListRow
                iconTile
                icon="checklist"
                title={t('help:more.setup_guide')}
                last={!help.feedback}
                onPress={() => router.push('/setup-guide')}
              />
            ) : null}
            {help.feedback ? (
              <ListRow
                iconTile
                icon="feedback"
                title={t('help:more.feedback')}
                onPress={() => void openFeedback({ route: '/more', capture: false })}
                last
              />
            ) : null}
          </List>
        </GuideTarget> : null}
        <GuideTarget id="more.profile">
          <List>
            <ListRow
              iconTile
              leading={<Avatar name={name} size={44} dot={PRESENCE_DOT[presence.data?.status ?? ''] ?? undefined} />}
              title={name}
              subtitle={user?.email}
            />
            <ListRow
              iconTile
              icon="building"
              title={org?.name ?? ''}
              subtitle={orgCount > 1 ? t('more.switch_org', { count: orgCount }) : org ? roleLabel(org.role) : ''}
              onPress={orgCount > 1 ? () => router.push('/organizations') : undefined}
              last
            />
          </List>
        </GuideTarget>
        {groups.map((g, groupIndex) => {
          const items = g.items.filter((i) => i.show);
          if (!items.length) return null;
          return (
            <GuideTarget key={g.title} id={`more.${['find', 'team', 'work', 'admin', 'app'][groupIndex]}`}>
              <SectionHeader title={g.title} />
              <List>
                {items.map((i, idx) => (
                  <ListRow
                    iconTile
                    key={i.key}
                    icon={i.icon}
                    title={t(`more.${i.key}`)}
                    trailing={
                      i.badge ? (
                        <View className="min-w-6 items-center rounded-full bg-signal px-1.5 py-0.5">
                          <Text variant="caption" className="font-sans-bold text-signal-foreground">
                            {i.badge > 99 ? '99+' : i.badge}
                          </Text>
                        </View>
                      ) : undefined
                    }
                    onPress={() => router.push(i.to)}
                    last={idx === items.length - 1}
                  />
                ))}
              </List>
            </GuideTarget>
          );
        })}
        <List>
          <ListRow
            iconTile
            icon="externalLink"
            title={t('more.open_panel')}
            subtitle={t('more.open_panel_hint')}
            onPress={() => void WebBrowser.openBrowserAsync(WEB_URL)}
            last
          />
        </List>
      </Screen>
    </>
  );
}
