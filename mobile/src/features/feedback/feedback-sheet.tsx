import { useMutation } from '@tanstack/react-query';
import { Image } from 'expo-image';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Pressable, ScrollView, View } from 'react-native';

import { isApiError } from '@/core/api/errors';
import { P, useCan } from '@/core/auth/permissions';
import { APP_VERSION } from '@/core/config';
import { errorMessage } from '@/core/i18n';
import { Button } from '@/ui/button';
import { FormSheet } from '@/ui/form-sheet';
import { canMarkup, ImageMarkupEditor } from '@/features/inbox/thread/image-markup-editor';
import { SwitchRow } from '@/ui/fields';
import { Input } from '@/ui/input';
import { Text } from '@/ui/text';
import { toast } from '@/ui/toast';

import { deviceSummary, type FeedbackKind, sendFeedback } from './api';
import { closeFeedback, useFeedback } from './store';

const KINDS: FeedbackKind[] = ['bug', 'idea', 'other'];

/** The feedback sheet, opened by a shake, the header menu or More → "Send feedback". */
export function FeedbackSheet() {
  const open = useFeedback((s) => s.open);
  return open ? <FeedbackForm /> : null;
}

/**
 * Screenshot preview (removable), kind, note and what goes along (device,
 * app version, screen). The screenshot goes up through the attachments
 * upload first; a member without upload rights sends the text only.
 */
function FeedbackForm() {
  const { t } = useTranslation('help');
  const can = useCan();
  const screenshot = useFeedback((s) => s.screenshot);
  const [edited, setEdited] = useState<string | null>(null);
  const [markup, setMarkup] = useState(false);
  const preview = edited ?? screenshot;
  const route = useFeedback((s) => s.route);
  const [kind, setKind] = useState<FeedbackKind>('bug');
  const [message, setMessage] = useState('');
  const [withShot, setWithShot] = useState(true);
  const [includeDevice, setIncludeDevice] = useState(true);
  const canUpload = can(P.attachmentsWrite);
  const shot = preview && withShot && canUpload ? preview : null;
  const send = useMutation({
    mutationFn: () => sendFeedback({ message: message.trim(), kind, route, screenshot: shot, includeDevice }),
    onSuccess: () => {
      toast.success(t('feedback.sent'));
      closeFeedback();
    },
    onError: (e) =>
      toast.error(
        isApiError(e) && e.status === 404
          ? t('feedback.unavailable')
          : errorMessage(isApiError(e) ? e.code : undefined),
      ),
  });
  const ready = message.trim().length > 0 && !send.isPending;

  return (
    <FormSheet
      visible
      title={t('feedback.title')}
      onCancel={closeFeedback}
      dirty={!!message.trim() || !!edited}
      primary={
        markup
          ? undefined
          : { label: t('feedback.send'), loading: send.isPending, disabled: !ready, onPress: () => send.mutate() }
      }>
      {markup && preview ? (
        <ImageMarkupEditor
          file={{ uri: preview, name: 'feedback.jpg', type: 'image/jpeg' }}
          onCancel={() => setMarkup(false)}
          onDone={(file) => {
            setEdited(file.uri);
            setMarkup(false);
          }}
        />
      ) : (
        <ScrollView
          className="bg-background"
          keyboardShouldPersistTaps="handled"
          contentContainerClassName="gap-5 p-5 pb-10">
          {screenshot && canUpload ? (
            <View className="flex-row items-start gap-3">
              {withShot ? (
                <Pressable
                  accessibilityRole="button"
                  accessibilityLabel={t('feedback.markup')}
                  disabled={!canMarkup}
                  onPress={() => setMarkup(true)}>
                  <Image
                    source={{ uri: preview! }}
                    contentFit="cover"
                    accessibilityLabel={t('feedback.screenshot')}
                    className="rounded-lg border border-border"
                    style={{ width: 112, height: 224, borderRadius: 14 }}
                  />
                  <Text variant="caption" tone="muted" className="pt-2 text-center">
                    {t('feedback.markup')}
                  </Text>
                </Pressable>
              ) : null}
              <View className="flex-1 gap-2">
                <Text variant="footnote" tone="muted">
                  {withShot ? t('feedback.screenshot_hint') : t('feedback.screenshot_removed')}
                </Text>
                <Button
                  variant="outline"
                  icon={withShot ? 'trash' : 'photo'}
                  title={withShot ? t('feedback.remove_screenshot') : t('feedback.add_screenshot')}
                  onPress={() => setWithShot((v) => !v)}
                />
              </View>
            </View>
          ) : null}
          <View className="gap-1.5">
            <Text variant="footnote" tone="muted" className="font-sans-medium">
              {t('feedback.kind')}
            </Text>
            <View accessibilityRole="tablist" className="flex-row rounded-xl bg-secondary p-1">
              {KINDS.map((value) => (
                <Pressable
                  key={value}
                  accessibilityRole="tab"
                  accessibilityState={{ selected: value === kind }}
                  onPress={() => setKind(value)}
                  className={`min-h-10 flex-1 items-center justify-center rounded-lg ${value === kind ? 'bg-card' : ''}`}>
                  <Text variant="footnote" className="font-sans-semibold">
                    {t(`feedback.kind_${value}`)}
                  </Text>
                </Pressable>
              ))}
            </View>
          </View>
          <Input
            label={t('feedback.message')}
            placeholder={t(`feedback.placeholder_${kind}`)}
            value={message}
            onChangeText={setMessage}
            multiline
            textAlignVertical="top"
            maxLength={4000}
            className="min-h-32 py-3"
          />
          <SwitchRow
            label={t('feedback.include_device')}
            description={deviceSummary()}
            value={includeDevice}
            onChange={setIncludeDevice}
          />
          <Text variant="footnote" tone="muted">
            {t('feedback.privacy', { route: route || '/', version: APP_VERSION })}
          </Text>
        </ScrollView>
      )}
    </FormSheet>
  );
}
