import * as Clipboard from 'expo-clipboard';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ActivityIndicator, Pressable, ScrollView, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { isApiError } from '@/core/api/errors';
import { errorMessage } from '@/core/i18n';
import { Button } from '@/ui/button';
import { Chips } from '@/ui/chips';
import { cn } from '@/ui/cn';
import { FormSheet } from '@/ui/form-sheet';
import { Text } from '@/ui/text';
import { toast } from '@/ui/toast';

import { availableActions, type CopilotAction, needsDraft, resultTarget, TRANSLATE_LOCALES } from './actions';
import { useCopilot } from './copilot';

type Locale = (typeof TRANSLATE_LOCALES)[number];

/** API refusals the copilot explains in its own words; everything else uses the error catalog. */
function copilotError(e: unknown, t: (k: string) => string) {
  if (isApiError(e)) {
    if (e.status === 429) return t('error_credits');
    if (e.status === 503) return t('error_unavailable');
    if (e.status === 0) return t('error_offline');
    return errorMessage(e.code);
  }
  return errorMessage(undefined);
}

/**
 * The copilot from the composer (CF-82): suggest a reply, summarize the
 * conversation, translate / shorten / rewrite the draft. The result is
 * shown first; only the agent's tap puts it in the composer (or a note).
 */
export function CopilotSheet({
  visible,
  conversation,
  draft,
  canNote,
  onClose,
  onUseDraft,
  onUseNote,
}: {
  visible: boolean;
  conversation: string;
  draft: string;
  canNote: boolean;
  onClose: () => void;
  onUseDraft: (text: string) => void;
  onUseNote: (text: string) => void;
}) {
  const { t, i18n } = useTranslation('copilot');
  const insets = useSafeAreaInsets();
  const copilot = useCopilot(conversation);
  const [action, setAction] = useState<CopilotAction | null>(null);
  const [locale, setLocale] = useState<Locale>(i18n.language === 'en' ? 'tr' : 'en');
  const [result, setResult] = useState<string | null>(null);

  const run = (a: CopilotAction, loc: Locale = locale) => {
    setAction(a);
    setResult(null);
    copilot.mutate(
      {
        operation: a,
        ...(needsDraft(a) ? { text: draft.trim() } : {}),
        // Translation target; otherwise the answer comes in the agent's language.
        locale: a === 'translate' ? loc : i18n.language,
      },
      {
        onSuccess: (r) => setResult(r.text.trim()),
        onError: (e) => toast.error(copilotError(e, t)),
      },
    );
  };

  const close = () => {
    copilot.reset();
    setResult(null);
    setAction(null);
    onClose();
  };

  return (
    <FormSheet visible={visible} title={t('title')} onCancel={close} detents={['large']}>
      <View className="flex-1 bg-background" style={{ paddingBottom: insets.bottom }}>
        <ScrollView contentContainerClassName="gap-4 p-4" keyboardShouldPersistTaps="handled">
          <View className="gap-2">
            {availableActions(draft).map((a) => (
              <Pressable
                key={a}
                accessibilityRole="button"
                accessibilityState={{ selected: action === a, disabled: copilot.isPending }}
                disabled={copilot.isPending}
                onPress={() => run(a)}
                className={cn(
                  'min-h-12 flex-row items-center gap-3 rounded-lg border px-4 py-3 active:bg-muted',
                  action === a ? 'border-primary bg-card' : 'border-border bg-card',
                )}>
                <View className="flex-1 gap-0.5">
                  <Text className="font-sans-semibold">{t(`action_${a}`)}</Text>
                  <Text variant="footnote" tone="muted">
                    {t(`action_${a}_hint`)}
                  </Text>
                </View>
                {copilot.isPending && action === a ? <ActivityIndicator /> : null}
              </Pressable>
            ))}
            {draft.trim() ? null : (
              <Text variant="footnote" tone="muted">
                {t('draft_hint')}
              </Text>
            )}
          </View>
          {action === 'translate' ? (
            <View className="gap-2">
              <Text variant="footnote" tone="muted">
                {t('translate_to')}
              </Text>
              <Chips<Locale>
                value={locale}
                onChange={(l) => {
                  setLocale(l);
                  run('translate', l);
                }}
                options={TRANSLATE_LOCALES.map((l) => ({ value: l, label: t(`lang_${l}`) }))}
              />
            </View>
          ) : null}
          {result && action ? (
            <View className="gap-3 rounded-lg border border-border bg-card p-4">
              <Text variant="caption" tone="muted">
                {t('result')}
              </Text>
              <Text selectable>{result}</Text>
              <View className="gap-2">
                {resultTarget(action) === 'draft' ? (
                  <Button
                    title={t('use_draft')}
                    icon="edit"
                    onPress={() => {
                      onUseDraft(result);
                      close();
                    }}
                  />
                ) : canNote ? (
                  <Button
                    title={t('use_note')}
                    icon="note"
                    onPress={() => {
                      onUseNote(result);
                      close();
                    }}
                  />
                ) : null}
                <Button
                  title={t('copy')}
                  icon="copy"
                  variant="secondary"
                  onPress={() => {
                    void Clipboard.setStringAsync(result);
                    toast.success(t('copied'));
                  }}
                />
              </View>
              <Text variant="footnote" tone="muted">
                {t('review_hint')}
              </Text>
            </View>
          ) : null}
        </ScrollView>
      </View>
    </FormSheet>
  );
}
