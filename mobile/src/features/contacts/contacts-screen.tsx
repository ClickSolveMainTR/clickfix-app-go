import { HeaderActions } from '@/features/guides/help-menu';
import { GuideTarget } from '@/ui/guide-target';
import { useCompactHeader } from '@/features/shell/compact-header';
import { router, Stack } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Pressable, View } from 'react-native';

import { formatRelative } from '@/core/format';
import { useDebounced } from '@/core/use-debounced';
import { Avatar } from '@/ui/avatar';
import { Chips } from '@/ui/chips';
import { cn } from '@/ui/cn';
import { ConnectionBanner } from '@/ui/connection-banner';
import { EmptyState } from '@/ui/empty-state';
import { HeaderButton } from '@/ui/header-button';
import { Icon } from '@/ui/icon';
import { PagedList } from '@/ui/paged-list';
import { SearchField } from '@/ui/search-field';
import { Text } from '@/ui/text';

import { type Company, type ContactRecord, useCompanyList, useContactList, usePeopleScope } from './api';
import { healthTone } from './health';

type Tab = 'contacts' | 'companies';

/**
 * Contacts tab (CF-79): people and companies with search, the way HubSpot
 * and Intercom put the CRM next to the inbox. A row opens the record.
 */
export function ContactsScreen() {
  const { t } = useTranslation('people');
  const { canRead } = usePeopleScope();
  const [tab, setTab] = useState<Tab>('contacts');
  const [text, setText] = useState('');
  const q = useDebounced(text, 300);
  const contacts = useContactList(tab === 'contacts' ? q : '');
  const companies = useCompanyList(tab === 'companies' ? q : '');
  const compactHeader = useCompactHeader(t('title'));

  if (!canRead) {
    return (
      <>
        <Stack.Screen options={{ ...compactHeader }} />
        <View className="flex-1 justify-center bg-background">
          <EmptyState icon="lockClosed" title={t('no_permission')} />
        </View>
      </>
    );
  }

  const header = (
    <View className="gap-3">
      <GuideTarget id="contacts.search">
        <SearchField
          value={text}
          onChangeText={setText}
          placeholder={tab === 'contacts' ? t('search_contacts') : t('search_companies')}
        />
      </GuideTarget>
      <GuideTarget id="contacts.tabs">
        <Chips<Tab>
          value={tab}
          onChange={setTab}
          options={[
            { value: 'contacts', icon: 'customers', label: t('contacts') },
            { value: 'companies', icon: 'building', label: t('companies') },
          ]}
        />
      </GuideTarget>
      <ConnectionBanner offlineText={t('common:offline_body')} className="rounded-lg" />
    </View>
  );

  return (
    <>
      <Stack.Screen
        options={{
          ...compactHeader,
          headerRight: () => (
            <HeaderActions>
              {<HeaderButton icon="search" label={t('hub:search.title')} onPress={() => router.push('/search')} />}
            </HeaderActions>
          ),
        }}
      />
      {tab === 'contacts' ? (
        <PagedList<ContactRecord>
          query={contacts}
          emptyIcon="customers"
          keyOf={(c) => c.uuid}
          empty={q ? t('no_match') : t('contacts_empty')}
          header={header}
          renderItem={(c, _i, last) => (
            <GuideTarget id="contacts.row" enabled={_i === 0}>
              <ContactRow contact={c} last={last} />
            </GuideTarget>
          )}
        />
      ) : (
        <PagedList<Company>
          query={companies}
          emptyIcon="building"
          keyOf={(c) => c.uuid}
          empty={q ? t('no_match') : t('companies_empty')}
          header={header}
          renderItem={(c, _i, last) => <CompanyRow company={c} last={last} />}
        />
      )}
    </>
  );
}

function ContactRow({ contact: c, last }: { contact: ContactRecord; last: boolean }) {
  const { t } = useTranslation('people');
  const name = c.name || c.email || c.phone || '—';
  const sub = [c.company?.name, c.email ?? c.phone].filter(Boolean).join(' · ');
  return (
    <Pressable
      accessibilityRole="button"
      accessibilityLabel={[name, sub, t('conversations_count', { count: c.conversation_count })].join(', ')}
      onPress={() => router.push(`/contact/${c.uuid}`)}
      className="w-full min-w-0 active:bg-muted">
      <View className={cn('flex-row items-center gap-3 px-4 py-3', !last && 'border-b border-border')}>
        <Avatar name={name} />
        <View className="min-w-0 flex-1 gap-0.5">
          <Text variant="callout" className="font-sans-semibold" numberOfLines={1}>
            {name}
          </Text>
          {sub ? (
            <Text variant="footnote" tone="muted" numberOfLines={1}>
              {sub}
            </Text>
          ) : null}
        </View>
        <View className="shrink-0 items-end gap-0.5">
          {c.last_seen_at ? (
            <Text variant="footnote" tone="muted">
              {formatRelative(c.last_seen_at)}
            </Text>
          ) : null}
          {c.blocked ? (
            <Text variant="caption" tone="danger">
              {t('blocked')}
            </Text>
          ) : null}
        </View>
      </View>
    </Pressable>
  );
}

function CompanyRow({ company: c, last }: { company: Company; last: boolean }) {
  const { t } = useTranslation('people');
  const score = c.health?.score;
  const sub = [c.domain, t('contacts_count', { count: c.contact_count })].filter(Boolean).join(' · ');
  return (
    <Pressable
      accessibilityRole="button"
      accessibilityLabel={[c.name, sub, score != null ? t('health_score', { score }) : null].filter(Boolean).join(', ')}
      onPress={() => router.push(`/company/${c.uuid}`)}
      className="w-full min-w-0 active:bg-muted">
      <View className={cn('flex-row items-center gap-3 px-4 py-3', !last && 'border-b border-border')}>
        <View className="size-10 items-center justify-center rounded-lg bg-secondary">
          <Icon name="building" size={18} tone="primary" />
        </View>
        <View className="min-w-0 flex-1 gap-0.5">
          <Text variant="callout" className="font-sans-semibold" numberOfLines={1}>
            {c.name}
          </Text>
          <Text variant="footnote" tone="muted" numberOfLines={1}>
            {sub}
          </Text>
        </View>
        <View className="shrink-0 items-end gap-1">
          {score != null ? (
            <View className={cn('rounded-full px-2 py-0.5', healthTone(score, c.health?.below_threshold).bg)}>
              <Text variant="caption" className="font-sans-bold tabular-nums">
                {score}
              </Text>
            </View>
          ) : null}
          {c.open_conversations > 0 ? (
            <Text variant="footnote" tone="muted">
              {t('open_count', { count: c.open_conversations })}
            </Text>
          ) : null}
        </View>
      </View>
    </Pressable>
  );
}
