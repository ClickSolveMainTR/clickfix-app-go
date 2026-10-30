import { StatusPill } from '@/ui/status-pill';
import { router, Stack, useLocalSearchParams } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { View } from 'react-native';

import { formatRelative } from '@/core/format';
import { useDebounced } from '@/core/use-debounced';
import { ListRow } from '@/ui/list';
import { PagedList } from '@/ui/paged-list';
import { SearchField } from '@/ui/search-field';

import { type ArticleListItem, useArticles } from './api';

/** Articles of one knowledge collection, searchable. */
export function CollectionScreen() {
  const { t } = useTranslation('hub');
  const { collection, name } = useLocalSearchParams<{ collection: string; name?: string }>();
  const [text, setText] = useState('');
  const q = useDebounced(text, 300);
  const list = useArticles(collection ?? null, q);
  return (
    <>
      <Stack.Screen options={{ title: name ?? t('kb.title') }} />
      <PagedList<ArticleListItem>
        query={list}
        keyOf={(a) => a.uuid}
        emptyIcon="book"
        empty={t('kb.empty')}
        header={
          <View>
            <SearchField value={text} onChangeText={setText} placeholder={t('kb.search')} />
          </View>
        }
        renderItem={(a, _i, last) => (
          <ListRow
            icon="file"
            title={a.title}
            subtitle={[
              a.visibility === 'internal' ? t('kb.internal') : t('kb.public'),
              formatRelative(a.updated_at),
            ].join(' · ')}
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
