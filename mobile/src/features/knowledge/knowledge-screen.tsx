import { router, Stack } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { View } from 'react-native';
import { useDebounced } from '@/core/use-debounced';
import { List, ListRow, SectionHeader } from '@/ui/list';
import { PagedList } from '@/ui/paged-list';
import { SearchField } from '@/ui/search-field';
import { StatusPill } from '@/ui/status-pill';
import { useArticles, useCollections, type ArticleListItem } from './api';
export function KnowledgeScreen() {
  const { t } = useTranslation('hub');
  const [text, setText] = useState('');
  const articles = useArticles(null, useDebounced(text, 250));
  const collections = useCollections();
  return (
    <>
      <Stack.Screen options={{ title: t('kb.title') }} />
      <PagedList<ArticleListItem>
        query={articles}
        keyOf={(a) => a.uuid}
        emptyIcon="book"
        empty={t('kb.empty')}
        header={
          <View className="gap-3">
            <SearchField value={text} onChangeText={setText} placeholder={t('kb.search')} />
            {!text.trim() && collections.data?.items.length ? (
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
                          pathname: '/more/collection',
                          params: { collection: c.uuid, name: c.name },
                        })
                      }
                      last={i === all.length - 1}
                    />
                  ))}
                </List>
              </>
            ) : null}
            <SectionHeader title={t('kb.latest')} />
          </View>
        }
        renderItem={(a, _i, last) => (
          <ListRow
            icon="file"
            title={a.title}
            trailing={
              <StatusPill
                label={t(a.published_version ? 'kb.published' : 'kb.draft')}
                tone={a.published_version ? 'done' : 'neutral'}
              />
            }
            onPress={() => router.push({ pathname: '/more/article', params: { uuid: a.uuid } })}
            last={last}
          />
        )}
      />
    </>
  );
}
