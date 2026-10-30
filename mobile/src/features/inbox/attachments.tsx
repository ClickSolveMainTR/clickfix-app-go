import { useAudioPlayer, useAudioPlayerStatus } from 'expo-audio';
import { Image } from 'expo-image';
import * as WebBrowser from 'expo-web-browser';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ActivityIndicator, Modal, Pressable, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { API_URL } from '@/core/config';
import { mediaSource } from '@/core/api/client';
import { isApiError } from '@/core/api/errors';
import { errorMessage } from '@/core/i18n';
import { cn } from '@/ui/cn';
import { Icon } from '@/ui/icon';
import { Text } from '@/ui/text';
import { toast } from '@/ui/toast';

import { type Message, signedAttachmentUrl } from './api';
import { attachmentKind, formatBytes } from './helpers';

type Item = Message['attachments'][number];

/**
 * Files on a timeline message (CF-80): image thumbnails that open full
 * screen, an inline player for voice messages and audio, and a row for
 * everything else that opens through a short-lived signed URL. Bytes always
 * come from the API (`download_url`), never from storage keys.
 */
export function MessageAttachments({ items, onPrimary }: { items: Item[]; onPrimary: boolean }) {
  // Optimistic bubbles have no download URL yet: show the names only.
  if (items.some((a) => !a.download_url)) {
    return (
      <View className="w-60 max-w-full gap-1 pt-1">
        {items.map((a) => (
          <FileRow key={a.uuid} item={a} onPrimary={onPrimary} />
        ))}
      </View>
    );
  }
  const images = items.filter((a) => attachmentKind(a.content_type) === 'image');
  const rest = items.filter((a) => attachmentKind(a.content_type) !== 'image');
  return (
    <View className="w-60 max-w-full gap-1.5 pt-1">
      {images.length ? (
        <View className="flex-row flex-wrap gap-1.5">
          {images.map((a) => (
            <ImageThumb key={a.uuid} item={a} />
          ))}
        </View>
      ) : null}
      {rest.map((a) =>
        attachmentKind(a.content_type) === 'audio' ? (
          <AudioRow key={a.uuid} item={a} onPrimary={onPrimary} />
        ) : (
          <FileRow key={a.uuid} item={a} onPrimary={onPrimary} />
        ),
      )}
    </View>
  );
}

function ImageThumb({ item }: { item: Item }) {
  const { t } = useTranslation('conversation');
  const [open, setOpen] = useState(false);
  const insets = useSafeAreaInsets();
  return (
    <>
      <Pressable
        accessibilityRole="imagebutton"
        accessibilityLabel={t('attachments.open_image', { name: item.filename })}
        onPress={() => setOpen(true)}
        className="w-40 max-w-full">
        <Image
          source={mediaSource(item.thumbnail_url ?? item.download_url)}
          className="w-full rounded-xl bg-muted"
          style={{ borderCurve: 'continuous', aspectRatio: 1 }}
          contentFit="cover"
          transition={150}
          recyclingKey={item.uuid}
        />
      </Pressable>
      <Modal visible={open} animationType="fade" onRequestClose={() => setOpen(false)} statusBarTranslucent>
        <View className="flex-1 bg-ink">
          <Image source={mediaSource(item.download_url)} style={{ flex: 1 }} contentFit="contain" />
          <View
            className="absolute inset-x-0 flex-row items-center justify-between px-4"
            style={{ top: insets.top + 8 }}>
            <Pressable
              accessibilityRole="button"
              accessibilityLabel={t('common:close')}
              hitSlop={10}
              onPress={() => setOpen(false)}
              className="size-10 items-center justify-center rounded-full bg-background/80">
              <Icon name="close" size={20} tone="foreground" />
            </Pressable>
            <OpenButton item={item} />
          </View>
        </View>
      </Modal>
    </>
  );
}

function OpenButton({ item }: { item: Item }) {
  const { t } = useTranslation('conversation');
  const [busy, setBusy] = useState(false);
  return (
    <Pressable
      accessibilityRole="button"
      accessibilityLabel={t('attachments.open')}
      hitSlop={10}
      disabled={busy}
      onPress={() => openAttachment(item.uuid, setBusy)}
      className="size-10 items-center justify-center rounded-full bg-background/80">
      {busy ? <ActivityIndicator /> : <Icon name="share" size={20} tone="foreground" />}
    </Pressable>
  );
}

/** Opens a file in the system browser (preview, share, save) through a signed URL. */
async function openAttachment(uuid: string, setBusy: (b: boolean) => void) {
  setBusy(true);
  try {
    const signed = await signedAttachmentUrl(uuid);
    await WebBrowser.openBrowserAsync(signed.url.startsWith('http') ? signed.url : `${API_URL}${signed.url}`);
  } catch (e) {
    toast.error(errorMessage(isApiError(e) ? e.code : undefined));
  } finally {
    setBusy(false);
  }
}

function FileRow({ item, onPrimary }: { item: Item; onPrimary: boolean }) {
  const { i18n } = useTranslation();
  const [busy, setBusy] = useState(false);
  const ready = !!item.download_url;
  const kind = attachmentKind(item.content_type);
  return (
    <Pressable
      accessibilityRole="button"
      accessibilityLabel={item.filename}
      disabled={!ready || busy}
      onPress={() => openAttachment(item.uuid, setBusy)}
      className={cn(
        'min-h-11 flex-row items-center gap-2 rounded-xl px-2.5 py-2 active:opacity-70',
        onPrimary ? 'bg-ink/10' : 'bg-muted',
      )}>
      {busy ? (
        <ActivityIndicator />
      ) : (
        <Icon
          name={kind === 'image' ? 'photo' : kind === 'video' ? 'play' : 'file'}
          size={18}
          tone={onPrimary ? 'primary-foreground' : 'muted-foreground'}
        />
      )}
      <View className="min-w-0 flex-1">
        <Text
          variant="footnote"
          numberOfLines={1}
          tone={onPrimary ? 'onPrimary' : 'default'}
          className="font-sans-medium">
          {item.filename}
        </Text>
        <Text variant="caption" tone={onPrimary ? 'onPrimary' : 'muted'}>
          {formatBytes(item.size_bytes, i18n.language)}
        </Text>
      </View>
    </Pressable>
  );
}

function clock(seconds: number) {
  const s = Math.max(0, Math.floor(seconds || 0));
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`;
}

/** Voice messages and audio files play inline; the source carries the API auth headers. */
function AudioRow({ item, onPrimary }: { item: Item; onPrimary: boolean }) {
  const { t } = useTranslation('conversation');
  const player = useAudioPlayer(mediaSource(item.download_url));
  const status = useAudioPlayerStatus(player);
  const progress = status.duration > 0 ? Math.min(1, status.currentTime / status.duration) : 0;
  const toggle = () => {
    if (status.playing) {
      player.pause();
      return;
    }
    if (status.duration > 0 && status.currentTime >= status.duration - 0.1) void player.seekTo(0);
    player.play();
  };
  return (
    <View
      className={cn(
        'min-h-11 w-full flex-row items-center gap-2.5 rounded-xl px-2 py-1.5',
        onPrimary ? 'bg-ink/10' : 'bg-muted',
      )}>
      <Pressable
        accessibilityRole="button"
        accessibilityLabel={status.playing ? t('attachments.pause') : t('attachments.play')}
        onPress={toggle}
        className="size-9 items-center justify-center rounded-full bg-primary">
        {!status.isLoaded ? (
          <ActivityIndicator />
        ) : (
          <Icon name={status.playing ? 'pause' : 'play'} size={18} tone="primary-foreground" />
        )}
      </Pressable>
      <View className="flex-1 gap-1">
        <View className={cn('h-1 overflow-hidden rounded-full', onPrimary ? 'bg-ink/20' : 'bg-border')}>
          <View className="h-1 bg-signal" style={{ width: `${progress * 100}%` }} />
        </View>
        <Text variant="caption" tone={onPrimary ? 'onPrimary' : 'muted'} className="tabular-nums">
          {status.playing || status.currentTime > 0
            ? `${clock(status.currentTime)} / ${clock(status.duration)}`
            : clock(status.duration)}
        </Text>
      </View>
    </View>
  );
}
