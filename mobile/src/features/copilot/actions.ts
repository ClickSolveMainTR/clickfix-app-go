/**
 * Which copilot actions the composer offers and what each does with the
 * result. Kept free of app imports so the rules run under `node --test`.
 */
export type CopilotAction = 'suggest_reply' | 'summarize' | 'translate' | 'shorten' | 'rewrite';

/** Languages offered for translation (BCP 47; the API accepts any). */
export const TRANSLATE_LOCALES = ['tr', 'en', 'de', 'ar', 'ru', 'fr', 'es'] as const;

/** Actions that work on the draft need text in the composer. */
export function needsDraft(a: CopilotAction): boolean {
  return a === 'translate' || a === 'shorten' || a === 'rewrite';
}

/** Actions available for a draft: everything that needs no text, plus the draft tools when there is one. */
export function availableActions(draft: string): CopilotAction[] {
  const all: CopilotAction[] = ['suggest_reply', 'summarize', 'translate', 'shorten', 'rewrite'];
  return all.filter((a) => !needsDraft(a) || draft.trim().length > 0);
}

/**
 * Where a result goes: a summary is read (or kept as an internal note), the
 * rest replaces the draft. `suggest_reply` on top of a draft also replaces
 * it; the agent asked for a new reply.
 */
export function resultTarget(a: CopilotAction): 'draft' | 'read' {
  return a === 'summarize' ? 'read' : 'draft';
}
