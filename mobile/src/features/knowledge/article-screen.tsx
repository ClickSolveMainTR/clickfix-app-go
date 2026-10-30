import { HeaderActions } from '@/features/guides/help-menu';
import { Stack, useLocalSearchParams } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { KeyboardController } from 'react-native-keyboard-controller';
import { Share, View } from 'react-native';

import { isApiError } from '@/core/api/errors';
import { useSession } from '@/core/auth/session';
import { WEB_URL } from '@/core/config';
import { formatShort } from '@/core/format';
import { errorMessage } from '@/core/i18n';
import { Button } from '@/ui/button';
import { Card } from '@/ui/card';
import { HeaderButton } from '@/ui/header-button';
import { Input } from '@/ui/input';
import { TextArea } from '@/ui/fields';
import { Markdown } from '@/ui/markdown';
import { markdownToText } from '@/ui/markdown-core';
import { QueryState } from '@/ui/query-state';
import { FormScreen, Screen } from '@/ui/screen';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';
import { toast } from '@/ui/toast';

import { type Article, useArticle, useKnowledgeScope, usePublishArticle, useSaveArticle } from './api';

/** Public help-center URL of an article (`/help/{org}/article/{slug}`), or the panel editor for internal ones. */
function articleUrl(a: Article, org: string) {
  return a.visibility === 'public' && a.published_version
    ? `${WEB_URL}/help/${org}/article/${a.slug}`
    : `${WEB_URL}/settings/knowledge/${a.uuid}`;
}

/**
 * One knowledge article: read it, share its link (help-center link for a
 * published public article), and, with edit rights, fix a draft's title or
 * text and publish it (CF-79). Rich editing stays on the panel.
 */
export function ArticleScreen() {
  const { t } = useTranslation('hub');
  const { uuid } = useLocalSearchParams<{ uuid: string }>();
  const article = useArticle(uuid);
  const org = useSession((s) => s.organization?.slug ?? '');
  const { canWrite } = useKnowledgeScope();
  const [editing, setEditing] = useState(false);
  const a = article.data;

  return (
    <>
      <Stack.Screen
        options={{
          title: a?.title ?? t('kb.title'),
          headerRight: () => (
            <HeaderActions>
              {a ? (
                <View className="flex-row items-center gap-1">
                  <HeaderButton
                    icon="share"
                    label={t('kb.share')}
                    onPress={() => void Share.share({ message: `${a.title}\n${articleUrl(a, org)}` })}
                  />
                  {canWrite && !editing ? (
                    <HeaderButton icon="edit" label={t('kb.edit')} onPress={() => setEditing(true)} />
                  ) : null}
                </View>
              ) : null}
            </HeaderActions>
          ),
        }}
      />
      {a && editing ? (
        <ArticleEditor article={a} onDone={() => setEditing(false)} />
      ) : (
        <Screen>
          <QueryState query={article}>
            {(x) => (
              <View className="gap-4">
                <View className="gap-2">
                  <View className="flex-row flex-wrap items-center gap-2">
                    <StatusPill
                      label={x.published_version ? t('kb.published') : t('kb.draft')}
                      tone={x.published_version ? 'done' : 'progress'}
                    />
                    <StatusPill label={x.visibility === 'internal' ? t('kb.internal') : t('kb.public')} />
                    <Text variant="footnote" tone="muted">
                      {t('kb.updated', { at: formatShort(x.updated_at) })}
                    </Text>
                  </View>
                  {x.published_version && x.version !== x.published_version ? (
                    <Text variant="footnote" tone="muted">
                      {t('kb.unpublished_changes')}
                    </Text>
                  ) : null}
                </View>
                <Markdown source={x.body} />
                {x.tags.length ? (
                  <Text variant="footnote" tone="muted">
                    {x.tags.map((tag) => `#${tag}`).join('  ')}
                  </Text>
                ) : null}
              </View>
            )}
          </QueryState>
        </Screen>
      )}
    </>
  );
}

function ArticleEditor({ article: a, onDone }: { article: Article; onDone: () => void }) {
  const { t } = useTranslation('hub');
  const [title, setTitle] = useState(a.title);
  const [body, setBody] = useState(a.body);
  const save = useSaveArticle(a.uuid);
  const publish = usePublishArticle(a.uuid);
  const changed = title.trim() !== a.title || body !== a.body;
  const fail = (e: unknown) => toast.error(errorMessage(isApiError(e) ? e.code : undefined));

  const submit = (andPublish: boolean) =>
    save.mutate(
      {
        slug: a.slug,
        locale: a.locale,
        title: title.trim(),
        body,
        tags: a.tags,
        visibility: a.visibility,
        version: a.version,
      },
      {
        onSuccess: (saved) => {
          if (!andPublish) {
            toast.success(t('kb.saved'));
            return onDone();
          }
          publish.mutate(saved.version, {
            onSuccess: () => {
              toast.success(t('kb.published_done'));
              onDone();
            },
            onError: fail,
          });
        },
        onError: fail,
      },
    );

  return (
    <FormScreen
      footer={
        <View className="flex-row gap-2">
          <Button title={t('common:cancel')} variant="secondary" className="flex-1" onPress={onDone} />
          <Button
            title={t('kb.save_draft')}
            variant="outline"
            className="flex-1"
            disabled={!changed || !title.trim()}
            loading={save.isPending && !publish.isPending}
            onPress={() => submit(false)}
          />
          <Button
            title={t('kb.publish')}
            className="flex-1"
            disabled={!title.trim()}
            loading={publish.isPending}
            onPress={() => submit(true)}
          />
        </View>
      }>
      <Card>
        <Text variant="footnote" tone="muted">
          {t('kb.edit_hint')}
        </Text>
      </Card>
      <Input
        returnKeyType="next"
        submitBehavior="submit"
        onSubmitEditing={() => KeyboardController.setFocusTo('next')}
        label={t('kb.field_title')}
        value={title}
        onChangeText={setTitle}
      />
      <TextArea label={t('kb.field_body')} value={body} onChangeText={setBody} multiline />
      <Text variant="footnote" tone="muted">
        {markdownToText(body).length} {t('kb.characters')}
      </Text>
    </FormScreen>
  );
}
