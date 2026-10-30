import { Stack } from 'expo-router';
import { useTranslation } from 'react-i18next';
import { Pressable, Switch } from 'react-native';

import { isApiError } from '@/core/api/errors';
import { displayTimeZone } from '@/core/format';
import { errorMessage } from '@/core/i18n';
import {
  channelOn,
  type NotificationEventType,
  type NotificationPreferences,
  type NotificationPreferencesUpdate,
  useNotificationEvents,
  useNotificationPreferences,
  useUpdateNotificationPreferences,
} from '@/features/push/preferences';
import { ListRow } from '@/ui/list';
import { ActionMenu } from '@/ui/menu';
import { Screen } from '@/ui/screen';
import { SettingsSection } from '@/ui/settings-rows';
import { QueryState } from '@/ui/query-state';
import { Text } from '@/ui/text';
import { toast } from '@/ui/toast';

const HOURS = Array.from({ length: 24 }, (_, h) => `${String(h).padStart(2, '0')}:00`);
// Every group of the backend catalog (notifications/usecase/preferences.go); a group missing
// here is not shown on the phone.
const GROUP_ORDER = ['inbox', 'sla', 'team', 'calendar', 'oncall', 'tickets', 'accounts', 'quality', 'jobs', 'billing'];

/**
 * Notification preferences on the phone (CF-81): phone pushes on / off, which
 * events push (the panel's matrix, push column; locked ones stay on) and quiet
 * hours. Texts come from the panel's account page (`web_auth`).
 */
export function NotificationPreferencesScreen() {
  const { t } = useTranslation('settings');
  const prefs = useNotificationPreferences();
  const events = useNotificationEvents();
  return (
    <>
      <Stack.Screen options={{ title: t('notification_prefs') }} />
      <Screen>
        <QueryState query={prefs}>{(p) => <Preferences prefs={p} events={events.data?.items ?? []} />}</QueryState>
      </Screen>
    </>
  );
}

function SwitchRow({
  title,
  hint,
  value,
  disabled,
  onChange,
  last,
}: {
  last?: boolean;
  title: string;
  hint?: string;
  value: boolean;
  disabled?: boolean;
  onChange: (v: boolean) => void;
}) {
  return (
    <ListRow
      last={last}
      title={title}
      titleLines={0}
      subtitle={hint}
      trailing={<Switch accessibilityLabel={title} value={value} disabled={disabled} onValueChange={onChange} />}
    />
  );
}

function Preferences({ prefs: p, events }: { prefs: NotificationPreferences; events: NotificationEventType[] }) {
  const { t } = useTranslation('web_auth');
  const { t: ts } = useTranslation('settings');
  const update = useUpdateNotificationPreferences();
  const save = (body: NotificationPreferencesUpdate) =>
    update.mutate(body, { onError: (e) => toast.error(errorMessage(isApiError(e) ? e.code : undefined)) });

  const groups = GROUP_ORDER.map((g) => ({ g, items: events.filter((e) => e.group === g) })).filter(
    (x) => x.items.length > 0,
  );
  const pushOff = !p.push_enabled || !p.inapp_enabled;

  return (
    <>
      <SettingsSection>
        <SwitchRow
          last
          title={ts('prefs_push')}
          hint={ts('prefs_push_hint')}
          value={p.push_enabled}
          disabled={update.isPending}
          onChange={(v) => save({ push_enabled: v, ...(v && !p.inapp_enabled ? { inapp_enabled: true } : {}) })}
        />
      </SettingsSection>
      {groups.map(({ g, items }) => (
        <SettingsSection key={g} title={t(`preferences.matrix.group_${g}`)}>
          {items.map((ev, index) => {
            const locked = ev.locked.includes('push');
            return (
              <SwitchRow
                key={ev.key}
                last={index === items.length - 1}
                title={t(`preferences.matrix.event_${ev.key}`, { defaultValue: ev.key })}
                hint={locked ? t('preferences.matrix.locked') : undefined}
                value={channelOn(p, ev, 'push')}
                disabled={locked || pushOff || update.isPending}
                onChange={(v) => save({ event_channels: { [ev.key]: { push: v } } })}
              />
            );
          })}
        </SettingsSection>
      ))}
      <SettingsSection title={t('preferences.quiet.title')}>
        <SwitchRow
          last={!p.quiet_hours.enabled}
          title={t('preferences.quiet.title')}
          hint={t('preferences.quiet.hint')}
          value={p.quiet_hours.enabled}
          disabled={update.isPending}
          onChange={(v) =>
            save({
              quiet_hours: {
                ...p.quiet_hours,
                enabled: v,
                // First time on the phone: the phone's zone.
                timezone: p.quiet_hours.timezone || displayTimeZone(),
              },
            })
          }
        />
        {p.quiet_hours.enabled ? (
          <>
            {(['start', 'end'] as const).map((field) => (
              <ActionMenu
                key={field}
                title={t(`preferences.quiet.${field}`)}
                fullWidth
                value={p.quiet_hours[field]}
                disabled={update.isPending}
                options={HOURS.map((h) => ({ value: h, label: h }))}
                onPick={(value) => save({ quiet_hours: { ...p.quiet_hours, [field]: value } })}
                trigger={
                  <Pressable accessibilityRole="button" disabled={update.isPending}>
                    <ListRow
                      title={t(`preferences.quiet.${field}`)}
                      trailing={<Text tone="muted">{p.quiet_hours[field]}</Text>}
                    />
                  </Pressable>
                }
              />
            ))}
            <Text variant="footnote" tone="muted" className="px-4 py-3">
              {t('preferences.quiet.timezone')}: {p.quiet_hours.timezone}
            </Text>
          </>
        ) : null}
      </SettingsSection>
    </>
  );
}
