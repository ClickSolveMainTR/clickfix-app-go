import { router, Stack, useLocalSearchParams } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Linking, RefreshControl, View } from 'react-native';

import { isApiError } from '@/core/api/errors';
import { formatDateTime } from '@/core/date-format';
import { appLocale, displayTimeZone, formatNumber } from '@/core/format';
import { useDebounced } from '@/core/use-debounced';
import { callTime } from '@/features/calls/calls-screen';
import { useMemberNames } from '@/features/team/names';
import { EmptyState } from '@/ui/empty-state';
import { List, ListRow, SectionHeader } from '@/ui/list';
import { PagedList } from '@/ui/paged-list';
import { QueryState } from '@/ui/query-state';
import { Screen } from '@/ui/screen';
import { SearchField } from '@/ui/search-field';
import { Text } from '@/ui/text';
import { useManualRefresh } from '@/ui/use-manual-refresh';

import {
  type FieldDefinition,
  type ObjectRecord,
  useObjectDefinitions,
  useObjectRecord,
  useObjectRecords,
  useObjectsScope,
  useObjectTypes,
} from './api';
import { activeDefinitions, plainText, valueView } from './core';

/** Shown instead of a screen the member may not read or that has nothing to show (also when opened by a link). */
function Notice({ title, body }: { title: string; body: string }) {
  return (
    <>
      <Stack.Screen options={{ title }} />
      <Screen>
        <EmptyState icon="grid" title={body} />
      </Screen>
    </>
  );
}

/**
 * Custom object types (CF-172): the kinds of records the organization keeps
 * (vehicles, contracts, devices…). Types and fields are defined on the
 * panel; here they are browsed. Needs `tenant.custom_fields.read` and
 * `tenant.custom_objects.read`.
 */
export function ObjectTypesScreen() {
  const { t } = useTranslation('admin');
  const { canRecords } = useObjectsScope();
  const types = useObjectTypes();
  const refresh = useManualRefresh(types);
  if (!canRecords) return <Notice title={t('objects.title')} body={t('objects.no_access')} />;
  return (
    <>
      <Stack.Screen options={{ title: t('objects.title') }} />
      <Screen refreshControl={<RefreshControl {...refresh} />}>
        <QueryState query={types}>
          {(d) => {
            const items = (d.items ?? []).filter((x) => !x.archived);
            return items.length ? (
              <List>
                {items.map((type, i) => (
                  <ListRow
                    key={type.uuid}
                    icon="grid"
                    title={type.plural_label || type.singular_label || type.key}
                    subtitle={type.description || undefined}
                    onPress={() => router.push({ pathname: '/more/object-records', params: { type: type.uuid } })}
                    last={i === items.length - 1}
                  />
                ))}
              </List>
            ) : (
              <EmptyState icon="grid" title={t('objects.empty')} body={t('objects.empty_hint')} />
            );
          }}
        </QueryState>
      </Screen>
    </>
  );
}

/** Formats one stored value as a line of text (list rows, and the detail's plain values). */
function useValueText() {
  const { t } = useTranslation('admin');
  const nameOf = useMemberNames();
  return (definition: FieldDefinition, value: unknown): string => {
    const v = valueView(definition, value);
    switch (v.kind) {
      case 'empty':
        return '';
      case 'options':
        return v.options
          .map((o) => (o.archived ? t('objects.archived_option', { label: o.label }) : o.label))
          .join(', ');
      case 'boolean':
        return v.value ? t('objects.yes') : t('objects.no');
      case 'money':
        try {
          return new Intl.NumberFormat(appLocale(), { style: 'currency', currency: v.currency }).format(v.amount);
        } catch {
          return `${formatNumber(v.amount, 2, 2)} ${v.currency}`;
        }
      case 'number':
        return formatNumber(v.value, v.precision);
      case 'date':
        return (
          formatDateTime(
            appLocale(),
            { day: 'numeric', month: 'short', year: 'numeric', timeZone: v.timeZone ?? displayTimeZone() },
            v.at,
          ) || plainText(value)
        );
      case 'user': {
        const name = nameOf(v.uuid);
        return name === '—' ? t('objects.unknown_member') : name;
      }
      default:
        return v.text;
    }
  };
}

/** Records of one object type: search and endless scroll; a row opens the record. */
export function ObjectRecordsScreen() {
  const { type } = useLocalSearchParams<{ type: string }>();
  const { t } = useTranslation('admin');
  const { canRecords } = useObjectsScope();
  const types = useObjectTypes();
  const definitions = useObjectDefinitions(type ?? '');
  const [text, setText] = useState('');
  const q = useDebounced(text, 300);
  const records = useObjectRecords(type ?? '', q);
  const valueText = useValueText();
  if (!canRecords) return <Notice title={t('objects.records')} body={t('objects.no_access')} />;
  const current = types.data?.items?.find((x) => x.uuid === type);
  // Opened without a type, or with one that is not in the organization (a broken link).
  if (!type || (types.data && !current))
    return <Notice title={t('objects.records')} body={t('objects.type_not_found')} />;
  const fields = activeDefinitions(definitions.data?.items ?? []);
  return (
    <>
      <Stack.Screen options={{ title: current?.plural_label || t('objects.records') }} />
      <PagedList<ObjectRecord>
        query={records}
        keyOf={(r) => r.uuid}
        emptyIcon="grid"
        empty={q.trim() ? t('common:no_results') : t('objects.records_empty')}
        header={<SearchField value={text} onChangeText={setText} placeholder={t('objects.search')} />}
        renderItem={(r, _i, last) => (
          <ListRow
            title={r.display_name || '—'}
            subtitle={
              // The first two filled fields, so rows with the same name can be told apart.
              fields
                .map((d) => ({ label: d.label, text: valueText(d, r.custom_fields?.[d.key]) }))
                .filter((x) => x.text)
                .slice(0, 2)
                .map((x) => `${x.label}: ${x.text}`)
                .join(' · ') || undefined
            }
            onPress={() => router.push({ pathname: '/more/object-record', params: { type, record: r.uuid } })}
            last={last}
          />
        )}
      />
    </>
  );
}

/** One record: every field of the type with its stored value; links open in the system apps. */
export function ObjectRecordScreen() {
  const { type, record } = useLocalSearchParams<{ type: string; record: string }>();
  const { t } = useTranslation('admin');
  const { canRecords } = useObjectsScope();
  const definitions = useObjectDefinitions(type ?? '');
  const query = useObjectRecord(type ?? '', record ?? '');
  const refresh = useManualRefresh(query);
  const valueText = useValueText();
  if (!canRecords) return <Notice title={t('objects.record')} body={t('objects.no_access')} />;
  // 404: no such record. 400: the id in the link is not a uuid (`record=abc`). Both mean "not found" here.
  const status = isApiError(query.error) ? query.error.status : 0;
  const missing = !type || !record || status === 404 || status === 400;
  if (missing) return <Notice title={t('objects.record')} body={t('objects.record_not_found')} />;
  const fields = activeDefinitions(definitions.data?.items ?? []);
  return (
    <>
      <Stack.Screen options={{ title: query.data?.display_name || t('objects.record') }} />
      <Screen refreshControl={<RefreshControl {...refresh} />}>
        <QueryState query={query}>
          {(r) => (
            <View className="gap-4">
              <List>
                <ListRow title={r.display_name || '—'} titleLines={3} subtitle={t('objects.name')} last />
              </List>
              <View>
                <SectionHeader title={t('objects.fields')} />
                <QueryState query={definitions} rows={3}>
                  {() => (
                    <List>
                      {fields.map((d, i) => {
                        const v = valueView(d, r.custom_fields?.[d.key]);
                        const text = valueText(d, r.custom_fields?.[d.key]) || '—';
                        return (
                          <ListRow
                            key={d.uuid}
                            title={d.label}
                            titleLines={2}
                            subtitle={
                              <Text
                                tone={v.kind === 'empty' ? 'muted' : v.kind === 'link' ? 'primary' : 'default'}
                                numberOfLines={v.kind === 'text' && v.multiline ? 12 : 4}>
                                {text}
                              </Text>
                            }
                            onPress={
                              v.kind === 'link' ? () => void Linking.openURL(v.href).catch(() => undefined) : undefined
                            }
                            last={i === fields.length - 1}
                          />
                        );
                      })}
                      {!fields.length ? <ListRow title={t('objects.no_fields')} last /> : null}
                    </List>
                  )}
                </QueryState>
              </View>
              <Text variant="footnote" tone="muted" className="px-4">
                {[
                  t('objects.created', { at: callTime(r.created_at) }),
                  t('objects.updated', { at: callTime(r.updated_at) }),
                  t('objects.read_only'),
                ].join('\n')}
              </Text>
            </View>
          )}
        </QueryState>
      </Screen>
    </>
  );
}
