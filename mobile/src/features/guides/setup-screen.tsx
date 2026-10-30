import { useCSSVariable } from 'uniwind';
import { Stack } from 'expo-router';
import { useTranslation } from 'react-i18next';
import { View } from 'react-native';
import Svg, { Circle } from 'react-native-svg';

import { Button } from '@/ui/button';
import { Card } from '@/ui/card';
import { EmptyState } from '@/ui/empty-state';
import { Icon } from '@/ui/icon';
import { List, ListRow } from '@/ui/list';
import { QueryState } from '@/ui/query-state';
import { Screen } from '@/ui/screen';
import { Text } from '@/ui/text';

import { useHelpAvailability, useSetupGuides } from './api';
import { type Guide, nextSetupGuide } from './core';
import { startGuide } from './store';

/**
 * Setup tour (CF-142): the organization's onboarding checklist (channels,
 * team invite, business hours, SLA, AI, widget, help center, the mobile
 * app ...) with its progress. Each item plays its guide; "Continue" plays
 * the next unfinished one and moves on by itself.
 */
export function SetupGuideScreen() {
  const { t } = useTranslation('help');
  const [track, progress] = useCSSVariable(['--color-secondary', '--color-success']);
  const available = useHelpAvailability((s) => s.guides);
  const setup = useSetupGuides();
  const next = setup.data ? nextSetupGuide(setup.data.items) : null;

  return (
    <>
      <Stack.Screen options={{ title: t('setup.title') }} />
      <Screen>
        {!available ? (
          <EmptyState icon="checklist" title={t('guides.unavailable')} />
        ) : (
          <QueryState query={setup}>
            {(data) =>
              data.items.length === 0 ? (
                <EmptyState icon="checklist" title={t('setup.empty')} />
              ) : (
                <>
                  <Card>
                    <View className="items-center gap-3 py-3">
                      <View
                        className="size-28 items-center justify-center"
                        accessibilityRole="progressbar"
                        accessibilityValue={{ min: 0, max: data.total, now: data.completed }}>
                        <Svg width={112} height={112} style={{ position: 'absolute' }}>
                          <Circle cx={56} cy={56} r={48} fill="none" stroke={typeof track === 'string' ? track : undefined} strokeWidth={8} />
                          <Circle
                            cx={56}
                            cy={56}
                            r={48}
                            fill="none"
                            stroke={typeof progress === 'string' ? progress : undefined}
                            strokeWidth={8}
                            strokeLinecap="round"
                            strokeDasharray={2 * Math.PI * 48}
                            strokeDashoffset={2 * Math.PI * 48 * (1 - data.completed / Math.max(data.total, 1))}
                            rotation={-90}
                            origin="56,56"
                          />
                        </Svg>
                        <Text variant="title" className="tabular-nums">
                          {Math.round((data.completed / Math.max(data.total, 1)) * 100)}%
                        </Text>
                      </View>
                      <Text variant="headline">{t('setup.progress', { done: data.completed, total: data.total })}</Text>
                    </View>
                    {next ? (
                      <Button
                        icon="start"
                        title={data.completed ? t('setup.continue') : t('setup.start')}
                        onPress={() => launch(next)}
                      />
                    ) : (
                      <Text variant="callout" tone="muted">
                        {t('setup.all_done')}
                      </Text>
                    )}
                  </Card>
                  <List>
                    {data.items.map((g, i) => (
                      <ListRow
                        key={g.key}
                        leading={<SetupMark guide={g} />}
                        title={g.title}
                        subtitle={g.summary || t(`setup.status_${g.progress.status}`)}
                        onPress={g.steps.length ? () => launch(g) : undefined}
                        last={i === data.items.length - 1}
                      />
                    ))}
                  </List>
                </>
              )
            }
          </QueryState>
        )}
      </Screen>
    </>
  );
}

function SetupMark({ guide }: { guide: Guide }) {
  const { t } = useTranslation('help');
  const s = guide.progress.status;
  return (
    <View
      accessible
      accessibilityLabel={t(`setup.status_${s}`)}
      className={`size-10 items-center justify-center rounded-xl ${s === 'completed' ? 'bg-success/10' : 'bg-secondary'}`}>
      <Icon
        name={s === 'completed' ? 'accept' : s === 'skipped' ? 'forward' : s === 'in_progress' ? 'timer' : 'start'}
        tone={s === 'completed' ? 'success' : s === 'in_progress' ? 'warning' : 'muted-foreground'}
        size={20}
      />
    </View>
  );
}

function launch(guide: Guide) {
  startGuide(guide, 'setup');
}
