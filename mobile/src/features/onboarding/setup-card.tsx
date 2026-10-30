import { useQuery } from '@tanstack/react-query';
import * as WebBrowser from 'expo-web-browser';
import { useTranslation } from 'react-i18next';
import { View } from 'react-native';

import { api, orgHeader, unwrap } from '@/core/api/client';
import { P, useCan } from '@/core/auth/permissions';
import { useSession } from '@/core/auth/session';
import { WEB_URL } from '@/core/config';
import { Button } from '@/ui/button';
import { Card } from '@/ui/card';
import { Text } from '@/ui/text';

const APPLYING = ['queued', 'running'];

/**
 * Smart set-up on the phone (CF-140), the concise version: owners and
 * admins see whether the AI-assisted set-up ran and, while it applies,
 * its progress. Discovery, the template gallery, the preview and the team
 * invites stay on the web panel (opened in the in-app browser): they need
 * room for a long checklist and a website field, and the panel's session
 * is where the new organization is opened.
 */
export function SetupCard() {
  const { t } = useTranslation('settings');
  const can = useCan();
  const org = useSession((s) => s.organization?.slug ?? '');
  const enabled = !!org && can(P.onboardingManage);
  const status = useQuery({
    queryKey: ['onboarding', org, 'state'],
    enabled,
    staleTime: 30_000,
    queryFn: () => unwrap(api.GET('/v1/tenant/onboarding', { params: { header: orgHeader() } })),
  });
  const latest = useQuery({
    queryKey: ['onboarding', org, 'setup-latest'],
    enabled,
    staleTime: 30_000,
    refetchInterval: (q) => (q.state.data?.run && APPLYING.includes(q.state.data.run.status) ? 3_000 : false),
    queryFn: () => unwrap(api.GET('/v1/tenant/onboarding/setup/latest', { params: { header: orgHeader() } })),
  });
  if (!enabled || !status.data || status.data.finished_at || !status.data.active || latest.isLoading) return null;
  const run = latest.data?.run ?? null;
  const applying = !!run && APPLYING.includes(run.status);
  const applied = !!run && !applying && run.status !== 'draft' && run.status !== 'undone';
  // Nothing to say once the set-up is applied and the undo window closed.
  if (applied && !run.can_undo) return null;
  const state =
    !run || run.status === 'undone' ? 'start' : applying ? 'applying' : run.status === 'draft' ? 'draft' : 'applied';
  const open = () =>
    void WebBrowser.openBrowserAsync(`${WEB_URL}/getting-started${run && state !== 'start' ? `?calisma=${run.uuid}` : ''}`);
  const total = Math.max(run?.total ?? 0, 1);
  return (
    <Card title={t('setup_title')}>
      <Text variant="callout">{t(`setup_${state}`)}</Text>
      {applying && run ? (
        <View className="gap-1">
          <View className="h-2 overflow-hidden rounded-full bg-muted">
            <View
              className="h-full rounded-full bg-primary"
              style={{ width: `${Math.round((run.done / total) * 100)}%` }}
            />
          </View>
          <Text variant="footnote" tone="muted">
            {t('setup_progress', { done: run.done, total: run.total })}
          </Text>
        </View>
      ) : null}
      <Button title={t(`setup_cta_${state}`)} icon="externalLink" variant="secondary" onPress={open} />
    </Card>
  );
}
