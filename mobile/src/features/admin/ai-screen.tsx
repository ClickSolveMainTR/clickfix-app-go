import { router, Stack } from 'expo-router';
import { useTranslation } from 'react-i18next';
import { View } from 'react-native';

import { isApiError } from '@/core/api/errors';
import { P, useCan } from '@/core/auth/permissions';
import { formatNumber } from '@/core/format';
import { errorMessage } from '@/core/i18n';
import { List, ListRow } from '@/ui/list';
import { Card, Stat } from '@/ui/card';
import { SwitchRow } from '@/ui/fields';
import { QueryState } from '@/ui/query-state';
import { Screen } from '@/ui/screen';
import { Text } from '@/ui/text';
import { toast } from '@/ui/toast';

import { useAIPolicy, useSetAIEnabled, useWallet } from './api';

/** AI for the organization: on / off (copilot, AI agent), limits, and the AI credit balance. */
export function AIScreen() {
  const { t } = useTranslation('admin');
  const can = useCan();
  const policy = useAIPolicy();
  const wallet = useWallet();
  const set = useSetAIEnabled();
  return (
    <>
      <Stack.Screen options={{ title: t('ai.title') }} />
      <Screen>
        <QueryState query={policy}>
          {(p) => (
            <View className="gap-4">
              <View>
                <SwitchRow
                  label={t('ai.enabled')}
                  description={t(p.enabled ? 'ai.state_on' : 'ai.state_off')}
                  value={p.enabled}
                  disabled={!can(P.settingsWrite) || set.isPending}
                  onChange={(v) =>
                    set.mutate(
                      { policy: p, enabled: v },
                      {
                        onSuccess: () => toast.success(v ? t('ai.on') : t('ai.off')),
                        onError: (e) => toast.error(errorMessage(isApiError(e) ? e.code : undefined)),
                      },
                    )
                  }
                />
              </View>
              <Card title={t('ai.limits')}>
                <View className="flex-row gap-3">
                  <Stat
                    label={t('ai.daily_calls')}
                    value={p.daily_calls ? formatNumber(p.daily_calls) : t('ai.unlimited')}
                  />
                  <Stat label={t('ai.concurrency')} value={formatNumber(p.concurrency)} />
                </View>
                <Text variant="footnote" tone="muted">
                  {[p.mask_pii ? t('ai.mask_pii') : null, t('ai.limits_hint')].filter(Boolean).join(' · ')}
                </Text>
              </Card>
            </View>
          )}
        </QueryState>
        {wallet.data ? (
          <Card title={t('ai.credits')}>
            <View className="flex-row gap-3">
              <Stat
                label={t('ai.credit_balance')}
                value={formatNumber(wallet.data.ai_credits)}
                tone="primary"
              />
              <Stat
                label={t('billing.wallet_try')}
                value={`${formatNumber(wallet.data.try_minor / 100, 2, 2)} ₺`}
              />
            </View>
            {can(P.billingRead) ? (
              <List>
                <ListRow
                  icon="wallet"
                  title={t('ai.buy')}
                  onPress={() => router.push('/more/billing')}
                  last
                />
              </List>
            ) : null}
            <Text variant="footnote" tone="muted">
              {t(`ai.when_empty_${wallet.data.ai_action}`)}
            </Text>
          </Card>
        ) : null}
      </Screen>
    </>
  );
}
