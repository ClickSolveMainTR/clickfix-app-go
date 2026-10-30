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

import { permissions } from "@/config/permissions";
import { usePermission } from "@/providers/permission-provider";

import { AssistantDrawer } from "./assistant-drawer";

type Pending = { prompt?: string; guideKey?: string; nonce: number };

type AssistantContextValue = {
  open: boolean;
  setOpen: (open: boolean) => void;
  toggle: () => void;
  /** Opens the panel; with a prompt, sends it as the user's question. */
  ask: (prompt?: string, guideKey?: string) => void;
};

const AssistantContext = createContext<AssistantContextValue | null>(null);

/**
 * ClickFix Asistan of the workspace shell (CF-141): a right-side panel on
 * every page (header button or Cmd/Ctrl + J). Members without
 * `tenant.assistant.use` get no panel.
 */
export function AssistantProvider({ children }: { children: ReactNode }) {
  const allowed = usePermission().can(permissions.assistant.use);
  const [open, setOpen] = useState(false);
  const [pending, setPending] = useState<Pending | null>(null);

  const toggle = useCallback(() => setOpen((o) => !o), []);
  const ask = useCallback((prompt?: string, guideKey?: string) => {
    setPending({ prompt, guideKey, nonce: Date.now() });
    setOpen(true);
  }, []);

  useEffect(() => {
    if (!allowed) return;
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "j") {
        e.preventDefault();
        setOpen((o) => !o);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [allowed]);

  const value = useMemo<AssistantContextValue>(
    () => ({ open, setOpen, toggle, ask }),
    [ask, open, toggle],
  );

  if (!allowed) return <>{children}</>;
  return (
    <AssistantContext.Provider value={value}>
      {children}
      <AssistantDrawer
        open={open}
        onOpenChange={setOpen}
        pending={pending}
        onPendingHandled={() => setPending(null)}
      />
    </AssistantContext.Provider>
  );
}

/** Assistant context, or null when the member has no assistant. */
export function useOptionalAssistant(): AssistantContextValue | null {
  return useContext(AssistantContext);
}
