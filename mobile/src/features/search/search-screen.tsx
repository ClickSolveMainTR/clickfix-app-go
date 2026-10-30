import { type Href, router, Stack } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ActivityIndicator, Pressable, SectionList, View } from 'react-native';

import { useDebounced } from '@/core/use-debounced';
import { useCollections, useArticles, useKnowledgeScope } from '@/features/knowledge/api';
import { appRoute } from '@/features/push/links';
import { Chips } from '@/ui/chips';
import { cn } from '@/ui/cn';
import { ConnectionBanner } from '@/ui/connection-banner';
import { EmptyState } from '@/ui/empty-state';
import { Icon } from '@/ui/icon';
import type { IconName } from '@/ui/icons';
import { List, ListRow, SectionHeader } from '@/ui/list';
import { SearchField } from '@/ui/search-field';
import { Skeleton } from '@/ui/skeleton';
import { Text } from '@/ui/text';

import {
  type ArticleHit,
  clearRecentSearches,
  recentSearches,
  rememberSearch,
  type SearchHit,
  type SearchSpec,
  useArticleSearch,
  useGlobalSearch,
} from './api';

type Scope = 'all' | SearchSpec | 'articles';
type Row = {
  key: string;
  icon: IconName;
  title: string;
  subtitle?: string | null;
  snippet?: string | null;
  to: string | null;
};

const SPEC_ICON: Record<string, IconName> = {
  conversations: 'inbox',
  messages: 'reply',
  contacts: 'user',
  companies: 'building',
};
const ORDER = ['conversations', 'messages', 'contacts', 'companies', 'articles'];

/** Where a hit opens: messages open their conversation, the rest map their panel link. */
function hitRoute(h: SearchHit): string | null {
  if (h.spec === 'messages' && h.conversation_id) return `/conversation/${h.conversation_id}`;
  const r = appRoute(h.href);
  return r && r !== '/' ? r : null;
}

/**
 * Search tab (CF-79): one field over conversations, messages, contacts,
 * companies and knowledge articles, like the panel's Cmd+K. Results group by
 * type; chips narrow to one. Without a query it shows the recent searches and
 * the knowledge base to browse.
 */
export function SearchScreen() {
  const { t } = useTranslation('hub');
  const [text, setText] = useState('');
  const [scope, setScope] = useState<Scope>('all');
  const [recent, setRecent] = useState(recentSearches);
  const q = useDebounced(text, 250);
  const { canRead: canKb } = useKnowledgeScope();
  const spec = scope === 'all' || scope === 'articles' ? null : scope;
  const hits = useGlobalSearch(scope === 'articles' ? '' : q, spec);
  const articles = useArticleSearch(q, scope === 'all' || scope === 'articles');
  const searching = q.trim().length >= 2;

  const label = (s: string) => t(`search.spec.${s}`);
  const groups = new Map<string, Row[]>();
  for (const h of hits.data?.items ?? []) {
    const rows = groups.get(h.spec) ?? [];
    rows.push({
      key: `${h.spec}:${h.id}`,
      icon: SPEC_ICON[h.spec] ?? 'search',
      title: h.title,
      subtitle: h.subtitle,
      snippet: h.snippet,
      to: hitRoute(h),
    });
    groups.set(h.spec, rows);
  }
  if (scope === 'all' || scope === 'articles')
    groups.set(
      'articles',
      (articles.data ?? []).map((a: ArticleHit) => ({
        key: `article:${a.uuid}`,
        icon: 'book' as const,
        title: a.title,
        snippet: a.snippet,
        to: `/article/${a.uuid}`,
      })),
    );
  const sections = ORDER.filter((s) => (groups.get(s)?.length ?? 0) > 0).map((s) => ({
    title: label(s),
    data: groups.get(s) ?? [],
  }));
  const failed =
    scope === 'articles' ? articles.isError : hits.isError || (scope === 'all' && articles.isError);
  const retry = () => {
    void hits.refetch();
    if (canKb) void articles.refetch();
  };
  const loading = (hits.isFetching || articles.isFetching) && sections.length === 0;

  const open = (row: Row) => {
    rememberSearch(text);
    setRecent(recentSearches());
    if (row.to) router.push(row.to as Href);
  };

  const scopes: Scope[] = [
    'all',
    'conversations',
    'messages',
    'contacts',
    'companies',
    ...(canKb ? (['articles'] as const) : []),
  ];

  return (
    <>
      <Stack.Screen options={{ title: t('search.title') }} />
      <SectionList
        sections={searching ? sections : []}
        keyExtractor={(r) => r.key}
        contentInsetAdjustmentBehavior="automatic"
        keyboardShouldPersistTaps="handled"
        keyboardDismissMode="interactive"
        automaticallyAdjustKeyboardInsets={false}
        stickySectionHeadersEnabled={false}
        className="flex-1 bg-background"
        contentContainerClassName="px-4 pb-10 pt-2"
        ListHeaderComponent={
          <View className="gap-3 pb-2">
            <SearchField value={text} onChangeText={setText} placeholder={t('search.placeholder')} />
            <Chips<Scope>
              value={scope}
              onChange={setScope}
              options={scopes.map((s) => ({
                value: s,
                label: label(s),
                icon: s === 'articles' ? 'book' : (SPEC_ICON[s] ?? 'search'),
              }))}
            />
            <ConnectionBanner offlineText={t('common:offline_body')} className="rounded-lg" />
          </View>
        }
        renderSectionHeader={({ section }) => <SectionHeader title={section.title} />}
        renderItem={({ item, index, section }) => (
          <ResultRow
            row={item}
            first={index === 0}
            last={index === section.data.length - 1}
            onPress={() => open(item)}
          />
        )}
        ListEmptyComponent={
          searching ? (
            failed ? (
              <EmptyState
                icon="error"
                title={t('common:load_failed')}
                action={{ title: t('common:retry'), icon: 'refresh', onPress: retry }}
              />
            ) : loading ? (
              <View className="gap-3 pt-2">
                {Array.from({ length: 5 }, (_, i) => (
                  <Skeleton key={i} className="h-14" />
                ))}
              </View>
            ) : (
              <EmptyState
                icon="search"
                title={t('search.no_results', { q: q.trim() })}
                body={t('search.no_results_hint')}
              />
            )
          ) : (
            <Browse
              recent={recent}
              onRecent={setText}
              onClearRecent={() => {
                clearRecentSearches();
                setRecent([]);
              }}
            />
          )
        }
        ListFooterComponent={
          hits.isFetching && sections.length > 0 ? <ActivityIndicator className="py-4" /> : undefined
        }
      />
    </>
  );
}

function ResultRow({
  row,
  first,
  last,
  onPress,
}: {
  row: Row;
  first: boolean;
  last: boolean;
  onPress: () => void;
}) {
  return (
    <Pressable
      accessibilityRole="button"
      accessibilityLabel={[row.title, row.subtitle, row.snippet].filter(Boolean).join(', ')}
      disabled={!row.to}
      onPress={onPress}
      className={cn(
        'bg-card active:bg-muted',
        first && 'rounded-t-lg',
        last ? 'rounded-b-lg' : '',
      )}>
      <View className={cn('flex-row items-start gap-3 px-4 py-3', !last && 'border-b border-border')}>
        <View className="pt-0.5">
          <Icon name={row.icon} size={18} tone="primary" />
        </View>
        <View className="flex-1 gap-0.5">
          <Text variant="callout" className="font-sans-semibold" numberOfLines={1}>
            {row.title}
          </Text>
          {row.subtitle ? (
            <Text variant="footnote" tone="muted" numberOfLines={1}>
              {row.subtitle}
            </Text>
          ) : null}
          {row.snippet ? (
            <Text variant="footnote" numberOfLines={2}>
              {row.snippet.replace(/<\/?mark>/g, '')}
            </Text>
          ) : null}
        </View>
      </View>
    </Pressable>
  );
}

/** Empty query: recent searches, then the knowledge base by collection. */
function Browse({
  recent,
  onRecent,
  onClearRecent,
}: {
  recent: string[];
  onRecent: (q: string) => void;
  onClearRecent: () => void;
}) {
  const { t } = useTranslation('hub');
  const { canRead } = useKnowledgeScope();
  const collections = useCollections();
  const latest = useArticles(null);
  const items = latest.data?.pages.flatMap((p) => p.items).slice(0, 8) ?? [];

  return (
    <View className="gap-1">
      {recent.length ? (
        <>
          <View className="flex-row items-center justify-between">
            <SectionHeader title={t('search.recent')} />
            <Pressable accessibilityRole="button" hitSlop={8} onPress={onClearRecent} className="px-4 pt-4">
              <Text variant="footnote" tone="primary" className="font-sans-semibold">
                {t('search.clear_recent')}
              </Text>
            </Pressable>
          </View>
          <List>
            {recent.map((r, i) => (
              <ListRow
                key={r}
                icon="history"
                title={r}
                onPress={() => onRecent(r)}
                last={i === recent.length - 1}
              />
            ))}
          </List>
        </>
      ) : null}
      {canRead ? (
        <>
          {collections.data?.items?.length ? (
            <>
              <SectionHeader title={t('kb.collections')} />
              <List>
                {collections.data.items.map((c, i, all) => (
                  <ListRow
                    key={c.uuid}
                    icon="book"
                    title={c.name}
                    onPress={() =>
                      router.push({
                        pathname: '/knowledge/[collection]',
                        params: { collection: c.uuid, name: c.name },
                      })
                    }
                    last={i === all.length - 1}
                  />
                ))}
              </List>
            </>
          ) : null}
          {items.length ? (
            <>
              <SectionHeader title={t('kb.latest')} />
              <List>
                {items.map((a, i) => (
                  <ListRow
                    key={a.uuid}
                    icon="note"
                    title={a.title}
                    subtitle={a.published_version ? t('kb.published') : t('kb.draft')}
                    onPress={() => router.push(`/article/${a.uuid}`)}
                    last={i === items.length - 1}
                  />
                ))}
              </List>
            </>
          ) : null}
        </>
      ) : null}
      {!recent.length && !canRead ? (
        <EmptyState icon="search" title={t('search.start')} body={t('search.start_hint')} />
      ) : null}
    </View>
  );
}
