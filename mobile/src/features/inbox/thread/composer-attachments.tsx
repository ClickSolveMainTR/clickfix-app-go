import { useEffect, useRef, useState } from 'react';
import { ActivityIndicator, Modal, Pressable, ScrollView, View } from 'react-native';
import { Image } from 'expo-image';
import Svg, { Circle } from 'react-native-svg';
import { useTranslation } from 'react-i18next';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { Icon } from '@/ui/icon';
import { Text } from '@/ui/text';
import { toast } from '@/ui/toast';
import { uploadAttachment, type LocalFile } from '../api';
import type { OutboxAttachment } from '../outbox-core';
import { canMarkup, ImageMarkupEditor } from './image-markup-editor';

export type ComposerUpload = {
  id: string;
  revision: number;
  file: LocalFile;
  done?: OutboxAttachment;
  failed?: boolean;
  progress?: number;
};
export function useComposerAttachments() {
  const { t } = useTranslation('conversation');
  const [items, setItems] = useState<ComposerUpload[]>([]);
  const current = useRef<ComposerUpload[]>([]);
  const alive = useRef(true);
  useEffect(() => {
    alive.current = true;
    return () => {
      alive.current = false;
    };
  }, []);
  function update(next: ComposerUpload[]) {
    current.current = next;
    if (alive.current) setItems(next);
  }
  function patch(item: ComposerUpload, data: Partial<ComposerUpload>) {
    if (!alive.current) return;
    update(current.current.map((x) => (x.id === item.id && x.revision === item.revision ? { ...x, ...data } : x)));
  }
  async function upload(item: ComposerUpload) {
    try {
      const a = await uploadAttachment(item.file, (progress) => patch(item, { progress }));
      patch(item, {
        done: { uuid: a.uuid, filename: a.filename, content_type: a.content_type, size_bytes: a.size_bytes },
      });
    } catch {
      patch(item, { failed: true });
    }
  }
  function add(files: LocalFile[]) {
    const accepted = files.slice(0, Math.max(0, 10 - current.current.length));
    if (accepted.length < files.length) toast.error(t('image.limit'));
    const added = accepted.map((file) => ({ id: `${Date.now()}-${Math.random()}`, revision: 0, file }));
    update([...current.current, ...added]);
    added.forEach((item) => void upload(item));
    return added.length > 0;
  }
  function replace(id: string, file: LocalFile) {
    const previous = current.current.find((x) => x.id === id);
    if (!previous) return;
    const item = { id, revision: previous.revision + 1, file };
    update(current.current.map((x) => (x.id === id ? item : x)));
    void upload(item);
  }
  return {
    items,
    add,
    replace,
    remove: (id: string) => update(current.current.filter((x) => x.id !== id)),
    retry: (id: string) => {
      const item = current.current.find((x) => x.id === id);
      if (item?.failed) replace(id, item.file);
    },
    clear: () => update([]),
    ready: items.flatMap((x) => (x.done ? [x.done] : [])),
    pending: items.some((x) => !x.done && !x.failed),
    failed: items.some((x) => x.failed),
  };
}
export function ComposerAttachmentTray({
  attachments,
  caption,
}: {
  attachments: ReturnType<typeof useComposerAttachments>;
  caption: string;
}) {
  const { t } = useTranslation('conversation');
  const insets = useSafeAreaInsets();
  const [preview, setPreview] = useState<string | null>(null);
  const [editing, setEditing] = useState(false);
  const selected = attachments.items.find((x) => x.id === preview);
  const close = () => {
    setPreview(null);
    setEditing(false);
  };
  if (!attachments.items.length) return null;
  return (
    <>
      <ScrollView
        horizontal
        keyboardShouldPersistTaps="always"
        showsHorizontalScrollIndicator={false}
        contentContainerStyle={{ paddingVertical: 6, gap: 10 }}
        style={{ flexGrow: 0 }}>
        {attachments.items.map((item) => (
          <View key={item.id} className="w-[76px] gap-1 pt-1.5">
            <Pressable
              accessibilityRole="button"
              accessibilityLabel={t('image.preview', { name: item.file.name })}
              onPress={() => {
                setPreview(item.id);
                setEditing(false);
              }}
              className="size-[76px] items-center justify-center overflow-hidden rounded-xl bg-secondary"
              style={{ borderCurve: 'continuous' }}>
              {item.file.type.startsWith('image/') ? (
                <Image source={{ uri: item.file.uri }} style={{ width: 76, height: 76 }} contentFit="cover" />
              ) : (
                <Icon name="file" size={32} tone="muted-foreground" />
              )}
              {!item.done && !item.failed ? (
                <View
                  className="absolute size-10 items-center justify-center rounded-full bg-ink/50"
                  accessibilityRole="progressbar"
                  accessibilityLabel={t('image.uploading')}
                  accessibilityValue={{ min: 0, max: 100, now: Math.round((item.progress ?? 0) * 100) }}>
                  {item.progress && item.progress < 1 ? (
                    <Svg width={36} height={36}>
                      <Circle cx={18} cy={18} r={14} stroke="white" strokeOpacity={0.25} strokeWidth={3} fill="none" />
                      <Circle
                        cx={18}
                        cy={18}
                        r={14}
                        stroke="white"
                        strokeWidth={3}
                        strokeDasharray={`${Math.PI * 28}`}
                        strokeDashoffset={Math.PI * 28 * (1 - item.progress)}
                        rotation={-90}
                        origin="18,18"
                        strokeLinecap="round"
                        fill="none"
                      />
                    </Svg>
                  ) : (
                    <ActivityIndicator color="white" />
                  )}
                </View>
              ) : null}
            </Pressable>
            <Pressable
              accessibilityRole="button"
              accessibilityLabel={t('inbox:composer.remove_file')}
              hitSlop={4}
              onPress={() => {
                attachments.remove(item.id);
                if (preview === item.id) close();
              }}
              className="absolute -right-1.5 top-0 size-6 items-center justify-center rounded-full bg-primary/85">
              <Icon name="close" size={12} tone="primary-foreground" />
            </Pressable>
            {item.failed ? (
              <Pressable
                accessibilityRole="button"
                onPress={() => attachments.retry(item.id)}
                className="min-h-11 justify-center">
                <Text variant="caption" tone="danger">
                  {t('image.retry')}
                </Text>
              </Pressable>
            ) : item.file.type.startsWith('image/') ? null : (
              // Images speak for themselves (as in Messages); files keep their name.
              <Text variant="caption" tone="muted" numberOfLines={1}>
                {item.file.name}
              </Text>
            )}
          </View>
        ))}
      </ScrollView>
      <Modal visible={!!selected} presentationStyle="fullScreen" animationType="slide" onRequestClose={close}>
        {selected && editing && canMarkup ? (
          <ImageMarkupEditor
            file={selected.file}
            onCancel={() => setEditing(false)}
            onDone={(file) => {
              attachments.replace(selected.id, file);
              setEditing(false);
            }}
          />
        ) : selected ? (
          <View className="flex-1 bg-background" style={{ paddingTop: insets.top, paddingBottom: insets.bottom }}>
            <View className="min-h-14 flex-row items-center justify-between px-4">
              <Pressable accessibilityRole="button" onPress={close} className="min-h-11 justify-center">
                <Text>{t('common:close')}</Text>
              </Pressable>
              {selected.file.type.startsWith('image/') && canMarkup ? (
                <Pressable
                  accessibilityRole="button"
                  onPress={() => setEditing(true)}
                  className="min-h-11 justify-center">
                  <Text>{t('image.edit')}</Text>
                </Pressable>
              ) : null}
              <Pressable
                accessibilityRole="button"
                onPress={() => {
                  attachments.remove(selected.id);
                  close();
                }}
                className="min-h-11 justify-center">
                <Text tone="danger">{t('image.remove')}</Text>
              </Pressable>
            </View>
            {selected.file.type.startsWith('image/') ? (
              <Image source={{ uri: selected.file.uri }} style={{ flex: 1 }} contentFit="contain" />
            ) : (
              <View className="flex-1 items-center justify-center gap-3">
                <Icon name="file" size={48} tone="muted-foreground" />
                <Text>{selected.file.name}</Text>
              </View>
            )}
            {caption ? (
              <ScrollView style={{ maxHeight: 120 }} contentContainerStyle={{ padding: 16 }}>
                <Text>{caption}</Text>
              </ScrollView>
            ) : null}
          </View>
        ) : null}
      </Modal>
    </>
  );
}
