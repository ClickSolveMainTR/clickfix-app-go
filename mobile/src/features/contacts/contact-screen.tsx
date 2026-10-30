import { HeaderActions } from '@/features/guides/help-menu';
import { channelIcon } from '@/ui/channel';
import { FormSheet, type FormField } from '@/ui/form-sheet';
import * as Clipboard from 'expo-clipboard';
import { router, Stack, useLocalSearchParams } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Linking, View } from 'react-native';

import { isApiError } from '@/core/api/errors';
import { formatShort } from '@/core/format';
import { errorMessage } from '@/core/i18n';
import { Avatar } from '@/ui/avatar';
import { IconButton } from '@/ui/button';
import { Card } from '@/ui/card';
import { HeaderButton } from '@/ui/header-button';
import { List, ListRow, SectionHeader } from '@/ui/list';
import { QueryState } from '@/ui/query-state';
import { Screen } from '@/ui/screen';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';
import { toast } from '@/ui/toast';

import {
  useContactRecord,
  useCustomFields,
  usePeopleScope,
  useRecordConversations,
  useUpdateContact,
  type ContactRecord,
} from './api';
import { CustomFieldsView } from './custom-fields';
import { formatPhone, identityLabel } from './display';
import { ConversationHistory } from './record-conversations';

/**
 * A contact (CF-79): who it is, how to reach them (call, e-mail, copy),
 * their identities on each channel, custom fields, notes and conversation
 * history. With `tenant.contacts.write` the fields can be edited here.
 */
export function ContactScreen() {
  const { t } = useTranslation('people');
  const { uuid } = useLocalSearchParams<{ uuid: string }>();
  const contact = useContactRecord(uuid);
  const { canWrite } = usePeopleScope();
  const [editing, setEditing] = useState(false);

  return (
    <>
      <Stack.Screen
        options={{
          title: contact.data?.name ?? t('title'),
          headerRight: () => (
            <HeaderActions>
              {contact.data && canWrite && !editing ? (
                <HeaderButton icon="edit" label={t('edit')} onPress={() => setEditing(true)} />
              ) : null}
            </HeaderActions>
          ),
        }}
      />
      {contact.data && editing ? (
        <ContactEditor contact={contact.data} onDone={() => setEditing(false)} />
      ) : (
        <Screen>
          <QueryState query={contact}>{(c) => <ContactBody contact={c} />}</QueryState>
        </Screen>
      )}
    </>
  );
}

function ContactBody({ contact: c }: { contact: ContactRecord }) {
  const { t } = useTranslation('people');
  const fields = useCustomFields('contact');
  const conversations = useRecordConversations({ contact: c.uuid });
  const name = c.name || c.email || c.phone || '—';
  const copy = async (v: string) => {
    await Clipboard.setStringAsync(v);
    toast.success(t('copied'));
  };

  return (
    <View className="gap-4">
      <View className="items-center gap-2 pt-2">
        <Avatar name={name} size={72} />
        <Text variant="title" className="text-center" selectable>
          {name}
        </Text>
        {c.job_title || c.company ? (
          <Text tone="muted" className="text-center">
            {[c.job_title, c.company?.name].filter(Boolean).join(' · ')}
          </Text>
        ) : null}
        <View className="flex-row flex-wrap justify-center gap-2">
          {c.blocked ? <StatusPill label={t('blocked')} tone="problem" /> : null}
          {c.channel_types.map((ch) => (
            <StatusPill key={ch} icon={channelIcon(ch)} label={t(`channels:types.${ch}`, { defaultValue: ch })} />
          ))}
        </View>
        <View className="flex-row gap-3 pt-1">
          {c.phone ? (
            <IconButton icon="phone" label={t('call')} onPress={() => void Linking.openURL(`tel:${c.phone}`)} />
          ) : null}
          {c.email ? (
            <IconButton icon="mail" label={t('email')} onPress={() => void Linking.openURL(`mailto:${c.email}`)} />
          ) : null}
          {c.company ? (
            <IconButton
              icon="building"
              label={c.company.name}
              onPress={() => router.push(`/company/${c.company?.uuid}`)}
            />
          ) : null}
        </View>
      </View>

      <List>
        <ListRow
          icon="mail"
          title={c.email ?? '—'}
          subtitle={t('fields.email')}
          onPress={c.email ? () => void copy(c.email ?? '') : undefined}
          trailing={<View />}
        />
        <ListRow
          icon="phone"
          title={c.phone ? formatPhone(c.phone) : '—'}
          subtitle={t('fields.phone')}
          onPress={c.phone ? () => void copy(c.phone ?? '') : undefined}
          trailing={<View />}
        />
        <ListRow
          icon="language"
          title={[c.locale, c.timezone].filter(Boolean).join(' · ') || '—'}
          subtitle={t('fields.locale')}
        />
        <ListRow
          icon="clock"
          title={c.last_seen_at ? formatShort(c.last_seen_at) : '—'}
          subtitle={t('fields.last_seen')}
          last
        />
      </List>

      {c.identities?.length ? (
        <Card title={t('identities')}>
          {c.identities.map((i) => (
            <View key={i.id} className="flex-row items-center gap-2">
              <StatusPill
                icon={channelIcon(i.channel_type)}
                label={t(`channels:types.${i.channel_type}`, { defaultValue: i.channel_type })}
              />
              <Text variant="footnote" selectable className="flex-1" numberOfLines={1}>
                {identityLabel(i, name)}
              </Text>
            </View>
          ))}
        </Card>
      ) : null}

      {Object.keys(c.custom_fields ?? {}).length || fields.data?.items.length ? (
        <Card title={t('custom_fields')}>
          <CustomFieldsView
            defs={fields.data?.items ?? []}
            values={(c.custom_fields ?? {}) as Record<string, unknown>}
          />
        </Card>
      ) : null}

      {c.notes ? (
        <Card title={t('fields.notes')}>
          <Text selectable>{c.notes}</Text>
        </Card>
      ) : null}

      <View className="gap-2">
        <SectionHeader title={t('conversations_count', { count: conversations.data?.total ?? c.conversation_count })} />
        <QueryState query={conversations} rows={2}>
          {(page) => <ConversationHistory items={page.items} emptyText={t('no_conversations')} />}
        </QueryState>
      </View>
    </View>
  );
}

function ContactEditor({ contact: c, onDone }: { contact: ContactRecord; onDone: () => void }) {
  const { t } = useTranslation('people');
  const fields = useCustomFields('contact');
  const update = useUpdateContact(c.uuid);
  const [form, setForm] = useState({
    name: c.name,
    email: c.email ?? '',
    phone: c.phone ?? '',
    job_title: c.job_title,
    notes: c.notes,
    blocked: c.blocked,
  });
  const [custom, setCustom] = useState<Record<string, unknown>>((c.custom_fields ?? {}) as Record<string, unknown>);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const set = <K extends keyof typeof form>(k: K, v: (typeof form)[K]) => setForm((f) => ({ ...f, [k]: v }));

  const save = () =>
    update.mutate(
      { ...form, custom_fields: custom },
      {
        onSuccess: () => {
          toast.success(t('saved'));
          onDone();
        },
        onError: (e) => {
          if (isApiError(e)) setErrors(e.fieldErrors());
          toast.error(errorMessage(isApiError(e) ? e.code : undefined));
        },
      },
    );

  const customFields: FormField[] = (fields.data?.items ?? [])
    .filter((d) => !d.archived)
    .map((d): FormField => {
      const value = custom[d.key];
      const base = { key: d.key, label: d.required ? `${d.label} *` : d.label, hint: d.description || undefined };
      const change = (next: unknown) => setCustom((cur) => ({ ...cur, [d.key]: next }));
      if (d.type === 'boolean') return { ...base, kind: 'toggle', value: value === true, onChange: change };
      if (d.type === 'select')
        return {
          ...base,
          kind: 'pick',
          value: typeof value === 'string' ? value : null,
          onChange: change,
          options: d.options.filter((o) => !o.archived).map((o) => ({ value: o.value, label: o.label })),
        };
      if (d.type === 'multiselect')
        return {
          ...base,
          kind: 'multi',
          values: Array.isArray(value) ? (value as string[]) : [],
          onChange: change,
          options: d.options.filter((o) => !o.archived).map((o) => ({ value: o.value, label: o.label })),
        };
      if (d.type === 'user') return { ...base, kind: 'info', value: value == null ? '—' : String(value) };
      return {
        ...base,
        kind: 'text',
        value: value == null ? '' : String(value),
        onChange: (text) => {
          if (d.type === 'number' || d.type === 'money') {
            const n = Number(text.replace(',', '.'));
            change(!text.trim() ? null : Number.isFinite(n) ? n : text);
          } else change(text);
        },
        multiline: d.type === 'textarea',
        keyboard:
          d.type === 'email'
            ? 'email'
            : d.type === 'phone'
              ? 'phone'
              : d.type === 'number' || d.type === 'money'
                ? 'number'
                : undefined,
      };
    });
  return (
    <FormSheet
      visible
      title={t('edit')}
      onCancel={onDone}
      dirty={
        JSON.stringify(form) !==
          JSON.stringify({
            name: c.name,
            email: c.email ?? '',
            phone: c.phone ?? '',
            job_title: c.job_title,
            notes: c.notes,
            blocked: c.blocked,
          }) || JSON.stringify(custom) !== JSON.stringify(c.custom_fields ?? {})
      }
      detents={['large']}
      primary={{ label: t('common:save'), onPress: save, disabled: !form.name.trim(), loading: update.isPending }}
      sections={[
        {
          fields: [
            ...(['name', 'email', 'phone', 'job_title', 'notes'] as const).map((key): FormField => ({
              key,
              kind: 'text',
              label: t(`fields.${key}`),
              value: form[key],
              onChange: (v) => set(key, v),
              error: errors[key],
              keyboard: key === 'email' ? 'email' : key === 'phone' ? 'phone' : undefined,
              multiline: key === 'notes',
            })),
            {
              key: 'blocked',
              kind: 'toggle',
              label: t('blocked'),
              hint: t('blocked_hint'),
              value: form.blocked,
              onChange: (v) => set('blocked', v),
            },
          ],
        },
        ...(customFields.length ? [{ title: t('custom_fields'), fields: customFields }] : []),
      ]}
    />
  );
}
