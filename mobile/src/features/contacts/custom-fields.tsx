import { useTranslation } from 'react-i18next';
import { View } from 'react-native';

import { formatNumber } from '@/core/format';
import { SwitchRow } from '@/ui/fields';
import { Input } from '@/ui/input';
import { MultiSelect } from '@/ui/multi-select';
import { Select } from '@/ui/select';
import { Text } from '@/ui/text';

import type { CustomFieldDefinition } from './api';

type Values = Record<string, unknown>;

/** Field types the phone edits; `user` stays read-only (needs the member picker of the panel). */
const EDITABLE = new Set([
  'text',
  'textarea',
  'number',
  'money',
  'date',
  'boolean',
  'select',
  'multiselect',
  'url',
  'email',
  'phone',
]);

/** A stored custom field value as text, with option labels for selects. */
export function fieldText(def: CustomFieldDefinition | undefined, v: unknown, yes: string, no: string): string {
  if (v == null || v === '') return '—';
  if (typeof v === 'boolean') return v ? yes : no;
  if (def?.type === 'select') return def.options.find((o) => o.value === v)?.label ?? String(v);
  if (def?.type === 'multiselect' && Array.isArray(v))
    return v.map((x) => def.options.find((o) => o.value === x)?.label ?? String(x)).join(', ');
  if ((def?.type === 'number' || def?.type === 'money') && typeof v === 'number')
    return `${formatNumber(v, def.config.precision ?? 2)}${def.type === 'money' && def.config.currency ? ` ${def.config.currency}` : ''}`;
  if (Array.isArray(v)) return v.join(', ');
  if (typeof v === 'object') return JSON.stringify(v);
  return String(v);
}

/** Read-only list of an entity's custom fields: defined ones in panel order, then unknown keys. */
export function CustomFieldsView({ defs, values }: { defs: CustomFieldDefinition[]; values: Values }) {
  const { t } = useTranslation('people');
  const known = defs.filter((d) => !d.archived).sort((a, b) => a.position - b.position);
  const extra = Object.keys(values).filter((k) => !known.some((d) => d.key === k));
  const rows = [
    ...known.map((d) => ({ key: d.key, label: d.label, value: fieldText(d, values[d.key], t('yes'), t('no')) })),
    ...extra.map((k) => ({ key: k, label: k, value: fieldText(undefined, values[k], t('yes'), t('no')) })),
  ].filter((r) => r.value !== '—' || known.some((d) => d.key === r.key));
  if (!rows.length) return null;
  return (
    <View className="gap-2.5">
      {rows.map((r) => (
        <View key={r.key} className="flex-row gap-3">
          <Text variant="footnote" tone="muted" className="w-2/5" numberOfLines={2}>
            {r.label}
          </Text>
          <Text variant="footnote" selectable className="flex-1">
            {r.value}
          </Text>
        </View>
      ))}
    </View>
  );
}

/** Edits custom field values; `onChange` gets the whole map back. */
export function CustomFieldsEditor({
  defs,
  values,
  onChange,
}: {
  defs: CustomFieldDefinition[];
  values: Values;
  onChange: (v: Values) => void;
}) {
  const set = (key: string, v: unknown) => onChange({ ...values, [key]: v });
  const fields = defs.filter((d) => !d.archived && EDITABLE.has(d.type)).sort((a, b) => a.position - b.position);
  return (
    <View className="gap-4">
      {fields.map((d) => {
        const v = values[d.key];
        const label = d.required ? `${d.label} *` : d.label;
        switch (d.type) {
          case 'boolean':
            return (
              <SwitchRow
                key={d.key}
                label={label}
                description={d.description || undefined}
                value={v === true}
                onChange={(x) => set(d.key, x)}
              />
            );
          case 'select':
            return (
              <Select<string>
                key={d.key}
                label={label}
                value={typeof v === 'string' ? v : null}
                options={d.options.filter((o) => !o.archived).map((o) => ({ value: o.value, label: o.label }))}
                onChange={(x) => set(d.key, x)}
              />
            );
          case 'multiselect':
            return (
              <MultiSelect<string>
                key={d.key}
                label={label}
                values={Array.isArray(v) ? (v as string[]) : []}
                options={d.options.filter((o) => !o.archived).map((o) => ({ value: o.value, label: o.label }))}
                onChange={(x) => set(d.key, x)}
              />
            );
          case 'number':
          case 'money':
            return (
              <Input
                key={d.key}
                label={label}
                hint={d.description || undefined}
                keyboardType="decimal-pad"
                value={v == null ? '' : String(v)}
                onChangeText={(x) => {
                  const n = Number(x.replace(',', '.'));
                  set(d.key, x.trim() === '' ? null : Number.isFinite(n) ? n : x);
                }}
              />
            );
          default:
            return (
              <Input
                key={d.key}
                label={label}
                hint={d.description || undefined}
                placeholder={d.type === 'date' ? 'YYYY-AA-GG' : d.config.placeholder}
                keyboardType={
                  d.type === 'email'
                    ? 'email-address'
                    : d.type === 'phone'
                      ? 'phone-pad'
                      : d.type === 'url'
                        ? 'url'
                        : 'default'
                }
                autoCapitalize={d.type === 'text' || d.type === 'textarea' ? 'sentences' : 'none'}
                multiline={d.type === 'textarea'}
                value={typeof v === 'string' ? v : v == null ? '' : String(v)}
                onChangeText={(x) => set(d.key, x)}
              />
            );
        }
      })}
    </View>
  );
}
