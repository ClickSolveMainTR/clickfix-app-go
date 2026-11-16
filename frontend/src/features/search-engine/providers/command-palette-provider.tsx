"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react";

import { onDesktopCommand } from "@/lib/desktop/bridge";

type CommandPaletteContextValue = {
  open: boolean;
  setOpen: (open: boolean) => void;
};

const CommandPaletteContext = createContext<CommandPaletteContextValue | null>(
  null,
);

export function CommandPaletteProvider({ children }: { children: ReactNode }) {
  const [open, setOpen] = useState(false);

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
        event.preventDefault();
        setOpen((current) => !current);
      }
    };
    window.addEventListener("keydown", onKeyDown);
    // ClickFix Masaüstü global shortcut (Ctrl/Cmd+Shift+K) opens the palette.
    const offDesktop = onDesktopCommand((command) => {
      if (command.type === "command-palette") setOpen(true);
    });
    return () => {
      window.removeEventListener("keydown", onKeyDown);
      offDesktop();
    };
  }, []);

  const setOpenStable = useCallback((next: boolean) => setOpen(next), []);

  const value = useMemo(
    () => ({
      open,
      setOpen: setOpenStable,
    }),
    [open, setOpenStable],
  );

  return (
    <CommandPaletteContext.Provider value={value}>
      {children}
    </CommandPaletteContext.Provider>
  );
}

export function useCommandPalette() {
  const ctx = useContext(CommandPaletteContext);
  if (!ctx) {
    throw new Error(
      "useCommandPalette must be used within CommandPaletteProvider",
    );
  }
  return ctx;
}
