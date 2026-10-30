import type { ReactElement } from 'react';

export type SheetProps = {
  visible: boolean;
  onClose: () => void;
  children: ReactElement;
};
