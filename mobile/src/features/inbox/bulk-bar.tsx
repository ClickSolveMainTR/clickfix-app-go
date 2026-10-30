import { Fragment, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Platform, PlatformColor, Pressable, useWindowDimensions } from 'react-native';
import { Stack } from 'expo-router';

import { ActionMenu } from '@/ui/menu';
import { isApiError } from '@/core/api/errors';
import { formatShort } from '@/core/format';
import { errorMessage } from '@/core/i18n';
import { Icon } from '@/ui/icon';
import { ICONS, type IconName } from '@/ui/icons';
import { OptionSheet } from '@/ui/select';
import { Text } from '@/ui/text';
import { toast } from '@/ui/toast';
import { undoBar } from '@/ui/undo-bar';

import { type BulkAction, useAgents, useBulk, useInboxScope, useTags, useUndoBulk } from './api';
import { SNOOZE_PRESETS, snoozeUntil, type SnoozePreset } from './helpers';

const STATUSES = ['open', 'pending', 'resolved', 'closed'] as const;
const PRIORITIES = ['urgent', 'high', 'normal', 'low'] as const;
const UNASSIGN = '__none';

type Picker = BulkAction | null;

/**
 * Native edit-mode toolbar (CF-79): assign,
 * status, priority, snooze, tag. Each action asks for its value in a sheet,
 * runs `POST /conversations/bulk` and offers "Undo" for a few seconds.
 */
export function BulkBar({ selected, onDone }: { selected: string[]; onDone: () => void }) {
  const { t } = useTranslation('inbox');
  const { fontScale } = useWindowDimensions();
  const { canManage, canAssign } = useInboxScope();
  const [picker, setPicker] = useState<Picker>(null);
  const bulk = useBulk();
  const undo = useUndoBulk();
  const agents = useAgents(picker === 'assign');
  const tags = useTags(picker === 'tag' || picker === 'untag');
  const n = selected.length;

  const run = (action: BulkAction, value: string) => {
    setPicker(null);
    bulk.mutate(
      {
        conversations: selected,
        action,
        value: action === 'snooze' ? snoozeUntil(value as SnoozePreset).toISOString() : value,
      },
      {
        onSuccess: (r) => {
          onDone();
          const msg = r.failed
            ? t('conversation:bulk.partial', { applied: r.applied, failed: r.failed })
            : t('conversation:bulk.done', { count: r.applied });
          undoBar.show(msg, () =>
            undo.mutate(r.operation, {
              onSuccess: () => toast.success(t('conversation:bulk.undone')),
              onError: (e) => toast.error(errorMessage(isApiError(e) ? e.code : undefined)),
            }),
          );
        },
        onError: (e) => toast.error(errorMessage(isApiError(e) ? e.code : undefined)),
      },
    );
  };

  const actions: { key: BulkAction; icon: IconName; label: string; show: boolean }[] = [
    { key: 'assign', icon: 'personAdd', label: t('conversation:bulk.assign'), show: canAssign },
    { key: 'status', icon: 'accept', label: t('conversation:bulk.status'), show: canManage },
    { key: 'priority', icon: 'priorityFlag', label: t('conversation:bulk.priority'), show: canManage },
    { key: 'snooze', icon: 'clock', label: t('header.snooze'), show: canManage },
    { key: 'tag', icon: 'tag', label: t('conversation:bulk.tag'), show: canManage },
  ];

  const options = (action: Picker = picker): { value: string; label: string; description?: string }[] => {
    switch (action) {
      case 'assign':
        return [
          { value: 'me', label: t('shortcuts.take') },
          { value: UNASSIGN, label: t('header.unassign') },
          ...(agents.data?.items ?? []).map((a) => ({ value: a.uuid, label: a.name, description: a.email })),
        ];
      case 'status':
        return STATUSES.map((s) => ({ value: s, label: t(`status.${s}`) }));
      case 'priority':
        return PRIORITIES.map((p) => ({ value: p, label: t(`priority.${p}`) }));
      case 'snooze':
        return SNOOZE_PRESETS.map((p) => ({
          value: p,
          label: t(`snooze.${p}`),
          description: formatShort(snoozeUntil(p).toISOString()),
        }));
      case 'tag':
      case 'untag':
        return (tags.data?.items ?? []).map((tg) => ({ value: tg.name, label: `#${tg.name}` }));
      default:
        return [];
    }
  };

  const available = actions.filter((a) => a.show);
  // At most four controls, including overflow; leave room for Dynamic Type.
  const capacity = fontScale > 1.2 ? 3 : 4;
  const visible = available.length > capacity ? available.slice(0, capacity - 1) : available;
  const overflow = available.slice(visible.length);
  const disabled = !n || bulk.isPending;
  const disabledColor = PlatformColor('tertiaryLabel');
  const trigger = (icon: IconName, label: string, onPress?: () => void) => (
    <Pressable
      accessibilityRole="button"
      accessibilityLabel={label}
      accessibilityState={{ disabled, busy: bulk.isPending }}
      disabled={disabled}
      onPress={onPress}
      className="min-h-11 min-w-14 items-center justify-center gap-0.5 px-2 py-1 active:opacity-60">
      <Icon
        name={icon}
        size={21}
        tone={disabled ? 'muted-foreground' : 'primary'}
        iosColor={disabled ? disabledColor : undefined}
      />
      <Text
        variant="caption"
        uppercase={false}
        numberOfLines={1}
        tone={disabled ? 'muted' : 'primary'}
        style={Platform.OS === 'ios' && disabled ? { color: disabledColor } : undefined}>
        {label}
      </Text>
    </Pressable>
  );

  return (
    <>
      <Stack.Toolbar placement="bottom">
        <Stack.Toolbar.Spacer />
        {visible.map((a) => (
          <Fragment key={a.key}>
            <Stack.Toolbar.View>
              {a.key === 'status' || a.key === 'priority' || a.key === 'snooze' ? (
                <ActionMenu
                  title={a.label}
                  trigger={trigger(a.icon, a.label)}
                  options={options(a.key)}
                  disabled={disabled}
                  onPick={(value) => run(a.key, value)}
                />
              ) : (
                trigger(a.icon, a.label, () => setPicker(a.key))
              )}
            </Stack.Toolbar.View>
            <Stack.Toolbar.Spacer />
          </Fragment>
        ))}
        {overflow.length ? (
          <>
            <Stack.Toolbar.View>
              <ActionMenu<BulkAction>
                title={t('conversation:bulk.more')}
                trigger={trigger('more', t('conversation:bulk.more'))}
                disabled={disabled}
                options={overflow.map((a) => ({
                  value: a.key,
                  label: a.label,
                  systemImage: ICONS[a.icon].sf,
                }))}
                onPick={setPicker}
              />
            </Stack.Toolbar.View>
            <Stack.Toolbar.Spacer />
          </>
        ) : null}
      </Stack.Toolbar>
      <OptionSheet<string>
        visible={!!picker}
        title={
          picker === 'untag' ? t('conversation:bulk.remove_tag') : (actions.find((a) => a.key === picker)?.label ?? '')
        }
        options={options()}
        searchable={picker === 'assign' || picker === 'tag' || picker === 'untag'}
        loading={agents.isFetching || tags.isFetching}
        footer={
          picker === 'tag' ? { label: t('conversation:bulk.remove_tag'), onPress: () => setPicker('untag') } : undefined
        }
        onClose={() => setPicker(null)}
        onPick={(v) => picker && run(picker, v === UNASSIGN ? '' : v)}
      />
    </>
  );
}
