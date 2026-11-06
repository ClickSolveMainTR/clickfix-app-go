import { Stack } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, RefreshControl, View } from 'react-native';

import { isApiError } from '@/core/api/errors';
import { errorMessage } from '@/core/i18n';
import { useDebounced } from '@/core/use-debounced';
import { type Canned, useCannedMutations, useCannedResponses, useInboxScope } from '@/features/inbox/api';
import { EmptyState } from '@/ui/empty-state';
import { type FormSection, FormSheet } from '@/ui/form-sheet';
import { HeaderButton } from '@/ui/header-button';
import { List, ListRow } from '@/ui/list';
import { QueryState } from '@/ui/query-state';
import { Screen } from '@/ui/screen';
import { SearchField } from '@/ui/search-field';
import { Text } from '@/ui/text';
import { toast } from '@/ui/toast';
import { useManualRefresh } from '@/ui/use-manual-refresh';

import {
  canEdit,
  type CannedForm,
  CANNED_VARIABLES,
  cleanShortcut,
  EMPTY_FORM,
  formOf,
  sameForm,
  validForm,
} from './core';

/**
 * Canned response management (CF-165), the phone version of the panel's
 * Replies page (`frontend/src/features/macros/components/replies-page.tsx`):
 * search, create, edit and delete with the same fields and the same rules.
 * `tenant.macros.read` opens the screen and covers the member's own
 * (personal) replies; replies shared with the organization are read-only
 * without `tenant.macros.write`. Macros stay on the panel.
 */
export function RepliesScreen() {
  const { t } = useTranslation('admin');
  const { canCanned, canCannedWrite } = useInboxScope();
  const [text, setText] = useState('');
  const q = useDebounced(text, 300);
  const list = useCannedResponses(q, true);
  const refresh = useManualRefresh(list);
  const m = useCannedMutations();

  // One sheet for "new" (target null) and "edit"; the form is filled when it opens.
  const [open, setOpen] = useState(false);
  const [target, setTarget] = useState<Canned | null>(null);
  const [form, setForm] = useState<CannedForm>(EMPTY_FORM);
  const start = (item: Canned | null) => {
    setTarget(item);
    setForm(item ? formOf(item) : EMPTY_FORM);
    setOpen(true);
  };
  const close = () => setOpen(false);
  const set = (patch: Partial<CannedForm>) => setForm((f) => ({ ...f, ...patch }));

  const editable = !target || canEdit(target, canCannedWrite);
  const busy = m.create.isPending || m.update.isPending || m.remove.isPending;
  const fail = (e: unknown) =>
    toast.error(
      isApiError(e, 'CANNED_SHORTCUT_EXISTS')
        ? t('replies.shortcut_taken')
        : errorMessage(isApiError(e) ? e.code : undefined),
    );

  const save = () => {
    const body = {
      title: form.title.trim(),
      shortcut: form.shortcut,
      category: form.category.trim(),
      content: form.content.trim(),
    };
    const done = {
      onSuccess: () => {
        toast.success(t('replies.saved'));
        close();
      },
      onError: fail,
    };
    if (target) m.update.mutate({ uuid: target.uuid, body }, done);
    else m.create.mutate({ ...body, scope: form.scope }, done);
  };

  const remove = (item: Canned) =>
    Alert.alert(t('replies.delete_title'), t('replies.delete_body', { title: item.title }), [
      { text: t('common:cancel'), style: 'cancel' },
      {
        text: t('replies.delete'),
        style: 'destructive',
        onPress: () =>
          m.remove.mutate(item.uuid, {
            onSuccess: () => {
              toast.success(t('replies.deleted'));
              close();
            },
            onError: fail,
          }),
      },
    ]);

  const sections: FormSection[] = [
    {
      fields: [
        {
          key: 'title',
          kind: 'text',
          label: t('replies.field_title'),
          value: form.title,
          onChange: (title) => set({ title }),
          disabled: !editable,
        },
        {
          key: 'shortcut',
          kind: 'text',
          label: t('replies.field_shortcut'),
          value: form.shortcut,
          onChange: (shortcut) => set({ shortcut: cleanShortcut(shortcut) }),
          disabled: !editable,
        },
        {
          key: 'category',
          kind: 'text',
          label: t('replies.field_category'),
          value: form.category,
          onChange: (category) => set({ category }),
          disabled: !editable,
        },
      ],
      footer: editable ? t('replies.shortcut_hint') : t('replies.read_only'),
    },
    {
      title: t('replies.field_content'),
      fields: [
        {
          key: 'content',
          kind: 'text',
          label: t('replies.field_content'),
          value: form.content,
          onChange: (content) => set({ content }),
          multiline: true,
          disabled: !editable,
        },
      ],
      footer: t('replies.variables_hint'),
    },
    ...(editable
      ? [
          {
            title: t('replies.variables'),
            fields: CANNED_VARIABLES.map((variable) => ({
              key: variable,
              kind: 'button' as const,
              label: variable,
              onPress: () => setForm((f) => ({ ...f, content: f.content + variable })),
            })),
          },
        ]
      : []),
    {
      fields: [
        !target && canCannedWrite
          ? {
              key: 'scope',
              kind: 'toggle' as const,
              label: t('replies.share_org'),
              value: form.scope === 'organization',
              onChange: (shared: boolean) => set({ scope: shared ? 'organization' : 'personal' }),
            }
          : {
              key: 'scope',
              kind: 'info' as const,
              label: t('replies.scope'),
              value: t(`replies.scope_${target?.scope ?? 'personal'}`),
            },
      ],
      footer: target ? undefined : t('replies.scope_hint'),
    },
    ...(target && editable
      ? [
          {
            fields: [
              {
                key: 'delete',
                kind: 'button' as const,
                label: t('replies.delete'),
                disabled: busy,
                onPress: () => remove(target),
              },
            ],
          },
        ]
      : []),
  ];

  if (!canCanned)
    return (
      <>
        <Stack.Screen options={{ title: t('replies.title') }} />
        <Screen>
          <EmptyState icon="note" title={t('replies.no_permission')} />
        </Screen>
      </>
    );

  return (
    <>
      <Stack.Screen
        options={{
          title: t('replies.title'),
          headerRight: () => <HeaderButton icon="add" label={t('replies.new')} onPress={() => start(null)} />,
        }}
      />
      <FormSheet
        visible={open}
        title={target ? target.title : t('replies.new')}
        onCancel={close}
        dirty={editable && !sameForm(form, target ? formOf(target) : EMPTY_FORM)}
        detents={['large']}
        primary={
          editable
            ? { label: t('common:save'), disabled: !validForm(form) || busy, loading: busy, onPress: save }
            : undefined
        }
        sections={sections}
      />
      <Screen refreshControl={<RefreshControl {...refresh} />}>
        <SearchField value={text} onChangeText={setText} placeholder={t('replies.search')} />
        <QueryState query={list}>
          {(data) =>
            data.items.length ? (
              <List>
                {data.items.map((c, i) => (
                  <ListRow
                    key={c.uuid}
                    icon="note"
                    title={c.title}
                    subtitle={
                      <View className="gap-0.5">
                        <Text variant="footnote" tone="muted" numberOfLines={1}>
                          {[
                            `/${c.shortcut}`,
                            t(`replies.scope_${c.scope}`),
                            c.category,
                            t('replies.used', { count: c.usage_count }),
                          ]
                            .filter(Boolean)
                            .join(' · ')}
                        </Text>
                        <Text variant="footnote" tone="muted" numberOfLines={2}>
                          {c.content}
                        </Text>
                      </View>
                    }
                    onPress={() => start(c)}
                    last={i === data.items.length - 1}
                  />
                ))}
              </List>
            ) : q.trim() ? (
              <EmptyState icon="search" title={t('common:no_results')} />
            ) : (
              <EmptyState
                icon="note"
                title={t('replies.empty')}
                body={t('replies.empty_hint')}
                action={{ title: t('replies.new'), icon: 'add', onPress: () => start(null) }}
              />
            )
          }
        </QueryState>
      </Screen>
    </>
  );
}
