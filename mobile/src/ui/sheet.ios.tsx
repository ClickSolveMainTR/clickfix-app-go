import { usePreferences } from '@/core/preferences';
import { BottomSheet, Group, Host, RNHostView } from '@expo/ui/swift-ui';
import { presentationDetents, presentationDragIndicator } from '@expo/ui/swift-ui/modifiers';
import type { SheetProps } from './sheet.types';

/** Compatibility for non-form content; new forms use FormSheet's native navigation and fields. */
export function Sheet({ visible, onClose, children }: SheetProps) {
  const theme = usePreferences((s) => s.theme);
  return (
    <Host
      style={{ position: 'absolute', width: 0, height: 0 }}
      useViewportSizeMeasurement
      colorScheme={theme === 'system' ? undefined : theme}>
      <BottomSheet
        isPresented={visible}
        onIsPresentedChange={(presented) => {
          if (!presented) onClose();
        }}>
        <Group modifiers={[presentationDetents(['medium', 'large']), presentationDragIndicator('visible')]}>
          <RNHostView>{children}</RNHostView>
        </Group>
      </BottomSheet>
    </Host>
  );
}
