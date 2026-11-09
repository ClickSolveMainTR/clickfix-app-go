"use client";

import { createContext, useContext } from "react";

import type { DecisionQueue } from "../lib/messages";

export type AssistantSurface = {
  slug: string;
  locale: string;
  /** Narrow layout (widget-like) versus the drawer. */
  compact: boolean;
  decisions: DecisionQueue;
  ensureThread: () => Promise<string>;
  /** Refetch the thread's documents (after an upload or removal). */
  refreshDocuments: () => void;
};

export const AssistantSurfaceContext = createContext<AssistantSurface | null>(
  null,
);

export function useAssistantSurface(): AssistantSurface {
  const value = useContext(AssistantSurfaceContext);
  if (!value) throw new Error("AssistantSurfaceContext is missing");
  return value;
}
