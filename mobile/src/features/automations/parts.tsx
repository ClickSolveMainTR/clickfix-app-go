import { useTranslation } from 'react-i18next';
import { Switch, View } from 'react-native';

import { isApiError } from '@/core/api/errors';
import { errorMessage } from '@/core/i18n';
import { Text } from '@/ui/text';
import { toast } from '@/ui/toast';

import type { ConditionLine, T } from './core';

/** `t` of the `admin` namespace in the shape the summary helpers take. */
export function useAutomationT(): T {
  const { t } = useTranslation('admin');
  return (key, params) => t(key, params ?? {});
}

/** The on/off switch; without the write permission (or for a draft) it is shown but cannot be moved. */
export function EnabledSwitch({
  label,
  value,
  disabled,
  onChange,
}: {
  label: string;
  value: boolean;
  disabled: boolean;
  onChange: (enabled: boolean) => void;
}) {
  return <Switch accessibilityLabel={label} value={value} disabled={disabled} onValueChange={onChange} />;
}

/** What to tell the member when the API refused the switch. */
export function toggleFailed(t: T, e: unknown) {
  const reason = errorMessage(isApiError(e) ? e.code : undefined);
  toast.error(`${t('automations.toggle_failed')} ${reason}`);
}

/** Condition and step summaries: one line each, members indented under their group. */
export function SummaryLines({ lines }: { lines: ConditionLine[] }) {
  if (!lines.length) return null;
  return (
    <View className="gap-0.5">
      {lines.map((l, i) => (
        <Text key={i} variant="footnote" tone="muted" style={{ paddingLeft: l.depth * 12 }}>
          {l.text}
        </Text>
      ))}
    </View>
  );
}
