import { Host, ModalBottomSheet, RNHostView } from '@expo/ui/jetpack-compose';
import { fillMaxSize } from '@expo/ui/jetpack-compose/modifiers';

import { usePreferences } from '@/core/preferences';

import type { SheetProps } from './sheet.types';

export function Sheet({ visible, onClose, children }: SheetProps) {
  const theme = usePreferences((s) => s.theme);
  if (!visible) return null;
  return (
    <Host
      style={{ position: 'absolute', width: 0, height: 0 }}
      colorScheme={theme === 'system' ? undefined : theme}>
      <ModalBottomSheet
        onDismissRequest={onClose}
        skipPartiallyExpanded={false}
        showDragHandle
        sheetGesturesEnabled>
        <RNHostView modifiers={[fillMaxSize()]}>{children}</RNHostView>
      </ModalBottomSheet>
    </Host>
  );
}
