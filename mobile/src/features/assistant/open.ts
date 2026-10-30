import { router } from 'expo-router';

/**
 * Opens ClickFix Asistan. `route` is the screen it was asked from (sent as
 * the thread's context), `guide` the guide step that sent the member there,
 * `prompt` a question to put in the composer.
 */
export function openAssistant(params: { route?: string; guide?: string; prompt?: string } = {}) {
  const clean = Object.fromEntries(Object.entries(params).filter(([, v]) => !!v)) as Record<string, string>;
  router.push({ pathname: '/assistant', params: clean });
}
