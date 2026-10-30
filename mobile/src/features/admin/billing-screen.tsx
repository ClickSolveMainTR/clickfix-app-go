import { Stack } from 'expo-router';
import * as WebBrowser from 'expo-web-browser';
import { useTranslation } from 'react-i18next';
import { RefreshControl, View } from 'react-native';

import { WEB_URL } from '@/core/config';
import { formatNumber, formatShort, formatCompactDate } from '@/core/format';
import { Button } from '@/ui/button';
import { Card, Stat } from '@/ui/card';
import { List, ListRow } from '@/ui/list';
import { useManualRefresh } from '@/ui/use-manual-refresh';
import { QueryState } from '@/ui/query-state';
import { Screen } from '@/ui/screen';
import { StatusPill, type StatusTone } from '@/ui/status-pill';
import { Text } from '@/ui/text';

import { useInvoices, usePaymentAvailability, useSubscription, useWallet, usePricing } from './api';

const SUB_TONE: Record<string, StatusTone> = {
  active: 'done',
  trialing: 'progress',
  past_due: 'problem',
  grace: 'problem',
  suspended: 'problem',
  pending_payment: 'progress',
};

/**
 * Billing summary (read-only): subscription, wallet and AI credits, recent
 * invoices. Invoice files and payments open on the panel (the PDF endpoint
 * needs the session, never a URL assembled in the app). When the in-app
 * browser closes, the summary reloads so a payment made there shows up.
 */
export function BillingScreen() {
  const { t } = useTranslation('admin');
  const sub = useSubscription();
  const pricing = usePricing();
  const wallet = useWallet();
  const invoices = useInvoices();
  const availability = usePaymentAvailability();
  const reload = () => Promise.all([sub.refetch(), wallet.refetch(), invoices.refetch(), availability.refetch()]);
  const refresh = useManualRefresh({
    isRefetching: sub.isRefetching || wallet.isRefetching || invoices.isRefetching,
    refetch: reload,
  });
  const openPanel = async () => {
    await WebBrowser.openBrowserAsync(`${WEB_URL}/billing`);
    void reload();
  };
  const notReady = availability.data && !availability.data.ready;
  const sandbox = availability.data?.card.ready && availability.data.card.mode === 'sandbox';
  const trialDays = sub.data
    ? Math.max(
        1,
        (Date.parse(sub.data.trial_ends_at ?? sub.data.period_end) - Date.parse(sub.data.period_start)) / 86400000,
      )
    : 1;
  const trialPercent = Math.max(0, Math.min(100, ((sub.data?.trial_days_remaining ?? 0) / trialDays) * 100));
  const money = (minor: number, currency: string) => `${formatNumber(minor / 100, 2, 2)} ${currency}`;
  return (
    <>
      <Stack.Screen options={{ title: t('billing.title') }} />
      <Screen refreshControl={<RefreshControl {...refresh} />}>
        <QueryState query={sub} rows={1}>
          {(s) =>
            s ? (
              <Card
                title={
                  pricing.data?.items.find((p) => p.versions.some((v) => v.uuid === s.version_uuid))?.name ??
                  t('billing.subscription')
                }>
                <View className="flex-row items-center gap-2">
                  <StatusPill label={t(`billing.status_${s.status}`)} tone={SUB_TONE[s.status] ?? 'neutral'} />
                  {s.cancel_at_period_end ? <StatusPill label={t('billing.cancels')} tone="progress" /> : null}
                </View>
                <Text variant="footnote" tone="muted">
                  {[
                    t('billing.period', {
                      from: formatCompactDate(s.period_start),
                      to: formatCompactDate(s.period_end),
                    }),
                    s.trial_days_remaining ? t('billing.trial_left', { count: s.trial_days_remaining }) : null,
                  ]
                    .filter(Boolean)
                    .join(' · ')}
                </Text>
                {s.status === 'trialing' ? (
                  <View
                    accessibilityRole="progressbar"
                    accessibilityLabel={t('billing.trial_left', { count: s.trial_days_remaining ?? 0 })}
                    accessibilityValue={{ min: 0, max: 100, now: Math.round(trialPercent) }}
                    className="h-2 overflow-hidden rounded-full bg-muted">
                    <View className="h-full rounded-full bg-signal" style={{ width: `${trialPercent}%` }} />
                  </View>
                ) : null}
              </Card>
            ) : (
              <Card>
                <Text tone="muted">{t('billing.no_subscription')}</Text>
              </Card>
            )
          }
        </QueryState>
        {wallet.data ? (
          <Card title={t('billing.wallet')}>
            <View className="flex-row gap-3">
              <Stat label={t('billing.wallet_try')} value={`${formatNumber(wallet.data.try_minor / 100, 2, 2)} ₺`} />
              <Stat label={t('ai.credit_balance')} value={formatNumber(wallet.data.ai_credits)} tone="primary" />
            </View>
            <Stat label={t('billing.video_minutes')} value={formatNumber(wallet.data.video_minutes)} />
          </Card>
        ) : null}
        {invoices.data?.items.length ? (
          <View className="gap-2">
            <Text variant="caption" tone="muted" className="px-1">
              {t('billing.invoices')}
            </Text>
            <List>
              {invoices.data.items.map((i, idx, all) => (
                <ListRow
                  key={i.uuid}
                  icon="receipt"
                  title={`${i.number ?? i.payment_reference} · ${money(i.total_minor, i.currency)}`}
                  subtitle={[
                    t(`billing.invoice_${i.status}`, { defaultValue: i.status }),
                    t('billing.due', { at: formatShort(i.due_at) }),
                  ].join(' · ')}
                  onPress={() => void openPanel()}
                  last={idx === all.length - 1}
                />
              ))}
            </List>
          </View>
        ) : null}
        {notReady ? (
          <Card>
            <Text tone="muted">{t('billing.payments_unavailable')}</Text>
          </Card>
        ) : null}
        {sandbox ? (
          <Text variant="footnote" tone="muted" className="px-1">
            {t('billing.payments_sandbox')}
          </Text>
        ) : null}
        <Button title={t('billing.manage')} variant="outline" icon="externalLink" onPress={() => void openPanel()} />
      </Screen>
    </>
  );
}
