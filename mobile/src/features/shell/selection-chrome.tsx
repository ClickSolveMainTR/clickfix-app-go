import { createContext, useCallback, useContext } from 'react';
import { useFocusEffect } from 'expo-router';

export const SelectionChromeContext = createContext<(hidden: boolean) => void>(() => {});

/** Restore the tab bar when leaving the list, including split-pane navigation. */
export function useSelectionChrome(selecting: boolean) {
  const setHidden = useContext(SelectionChromeContext);
  useFocusEffect(
    useCallback(() => {
      setHidden(selecting);
      return () => setHidden(false);
    }, [selecting, setHidden]),
  );
}
