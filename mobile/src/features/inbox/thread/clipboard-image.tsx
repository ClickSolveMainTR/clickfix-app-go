import * as Clipboard from 'expo-clipboard';
import * as FileSystem from 'expo-file-system/legacy';
import { useCallback, useEffect, useRef, useState } from 'react';
import { AppState, Pressable } from 'react-native';
import { useTranslation } from 'react-i18next';
import { Icon } from '@/ui/icon';
import { Text } from '@/ui/text';
import { toast } from '@/ui/toast';
import type { LocalFile } from '../api';
import {
  clipboardChanged,
  consumeClipboard,
  initialClipboardOffer,
  shouldOfferClipboard,
} from './image-editing';

export function useClipboardImage(enabled: boolean) {
  const [state, setState] = useState(initialClipboardOffer);
  const request = useRef(0);
  const detect = useCallback(async () => {
    if (!enabled) return;
    const sequence = ++request.current;
    try {
      const hasImage = await Clipboard.hasImageAsync();
      if (sequence === request.current) setState((s) => ({ ...s, hasImage }));
    } catch {
      /* Clipboard availability should never interrupt typing. */
    }
  }, [enabled]);
  useEffect(() => {
    if (!enabled) return;
    let active = true;
    const sub = Clipboard.addClipboardListener((event) => {
      if (active) {
        request.current++;
        setState((s) =>
          clipboardChanged(
            s,
            event.contentTypes.some((type) => type === 'image'),
          ),
        );
      }
    });
    const foreground = AppState.addEventListener('change', (s) => {
      if (s === 'active') void detect();
    });
    return () => {
      active = false;
      sub.remove();
      foreground.remove();
    };
  }, [enabled, detect]);
  return { state, detect, consume: (revision: number) => setState((s) => consumeClipboard(s, revision)) };
}

export function ClipboardImageOffer({
  clipboard,
  focused,
  onImage,
}: {
  clipboard: ReturnType<typeof useClipboardImage>;
  focused: boolean;
  onImage: (file: LocalFile) => boolean | void;
}) {
  const { t } = useTranslation('conversation');
  const [busy, setBusy] = useState(false);
  const lock = useRef(false);
  if (!shouldOfferClipboard(clipboard.state, focused)) return null;
  return (
    <Pressable
      accessibilityRole="button"
      accessibilityLabel={t('image.paste')}
      disabled={busy}
      className="mb-1 flex-row items-center gap-2 self-start rounded-full bg-secondary px-3 py-2"
      onPress={async () => {
        if (lock.current) return;
        lock.current = true;
        setBusy(true);
        try {
          // Only this explicit user action may request paste permission / image bytes.
          const image = await Clipboard.getImageAsync({ format: 'png' });
          if (!image || !FileSystem.cacheDirectory) return;
          const name = `clipboard-${Date.now()}.png`;
          const uri = FileSystem.cacheDirectory + name;
          await FileSystem.writeAsStringAsync(uri, image.data.replace(/^data:image\/[^;]+;base64,/, ''), {
            encoding: FileSystem.EncodingType.Base64,
          });
          const accepted = onImage({ uri, name, type: 'image/png' });
          if (accepted !== false) clipboard.consume(clipboard.state.revision);
        } catch {
          toast.error(t('attach.failed'));
        } finally {
          lock.current = false;
          setBusy(false);
        }
      }}>
      <Icon name="photo" size={24} tone="muted-foreground" />
      <Text variant="footnote">{t(busy ? 'common:loading' : 'image.paste')}</Text>
    </Pressable>
  );
}
