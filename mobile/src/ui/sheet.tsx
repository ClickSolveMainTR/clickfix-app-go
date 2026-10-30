import { Modal } from 'react-native';

import type { SheetProps } from './sheet.types';

/** Non-iOS fallback; iOS uses the SwiftUI sheet with medium/large detents. */
export function Sheet({ visible, onClose, children }: SheetProps) {
  return (
    <Modal
      visible={visible}
      animationType="slide"
      presentationStyle="formSheet"
      onRequestClose={onClose}
      onDismiss={onClose}>
      {children}
    </Modal>
  );
}
