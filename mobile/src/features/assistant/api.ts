import { fetch as expoFetch } from 'expo/fetch';
import { getTokens } from '@/core/auth/tokens';
import { API_URL } from '@/core/config';
import { i18n } from '@/core/i18n';
import { readSSE } from './sse';
import { ApiError, toApiError } from '@/core/api/errors';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { api, orgHeader, unwrap, unwrapVoid, refreshTokens } from '@/core/api/client';
import type { components } from '@/core/api/schema';
import { orgKey, useOrgSlug } from '@/core/api/org';
import { noteMissing, retryUnlessMissing, useHelpAvailability } from '@/features/guides/api';

import { appendMessages, type AssistantMessage, withActionStatus } from './core';

export type { AssistantMessage, AssistantStatus, AssistantThread, ProposedAction } from './core';

/**
 * ClickFix Asistan (CF-142 mobile, CF-141 API): the platform's own AI for
 * support and setup. Contract under `/v1/tenant/assistant/*`: `status`,
 * `threads` (list / create), `threads/{uuid}/messages` (list / send,
 * synchronous answer), `actions/{id}/confirm|reject`, `escalate`. Actions
 * run with the member's own permissions; the API enforces RBAC.
 */
export type ThreadContext = components['schemas']['AssistantContext'];

export const assistantKeys = {
  all: (org: string) => orgKey(org, 'assistant'),
  status: (org: string) => [...assistantKeys.all(org), 'status'] as const,
  threads: (org: string) => [...assistantKeys.all(org), 'threads'] as const,
  messages: (org: string, thread: string) => [...assistantKeys.all(org), 'messages', thread] as const,
};

export function useAssistantStatus() {
  const org = useOrgSlug();
  const available = useHelpAvailability((s) => s.assistant);
  return useQuery({
    queryKey: assistantKeys.status(org),
    enabled: !!org && available,
    staleTime: 60_000,
    retry: retryUnlessMissing,
    queryFn: async () => {
      try {
        return await unwrap(api.GET('/v1/tenant/assistant/status', { params: { header: orgHeader() } }));
      } catch (e) {
        noteMissing('assistant', e);
        throw e;
      }
    },
  });
}

export function useAssistantThreads(enabled: boolean) {
  const org = useOrgSlug();
  return useQuery({
    queryKey: assistantKeys.threads(org),
    enabled: !!org && enabled,
    staleTime: 30_000,
    queryFn: async () =>
      (await unwrap(api.GET('/v1/tenant/assistant/threads', { params: { header: orgHeader() } })))?.items ?? [],
  });
}

export function useThreadMessages(thread: string | null) {
  const org = useOrgSlug();
  return useQuery({
    queryKey: assistantKeys.messages(org, thread ?? ''),
    enabled: !!org && !!thread,
    staleTime: 15_000,
    queryFn: async () =>
      (
        await unwrap(
          api.GET('/v1/tenant/assistant/threads/{uuid}/messages', {
            params: { header: orgHeader(), path: { uuid: thread! } },
          }),
        )
      )?.items ?? [],
  });
}

type SendResult = { thread: string; message: AssistantMessage; reply: AssistantMessage };

/**
 * Sends a question; a new conversation gets its thread (with the screen it
 * was asked from as context) on the first message. The answer comes back
 * in the same call.
 */
export function useAskAssistant(context: ThreadContext) {
  const org = useOrgSlug();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async ({
      thread,
      content,
      signal,
      onEvent,
      onThread,
    }: {
      thread: string | null;
      content: string;
      signal?: AbortSignal;
      onEvent?: (name: string, data: unknown) => void;
      onThread?: (id: string) => void;
    }): Promise<SendResult> => {
      const id =
        thread ??
        (
          await unwrap(
            api.POST('/v1/tenant/assistant/threads', { params: { header: orgHeader() }, body: { context }, signal }),
          )
        ).uuid;
      onThread?.(id);
      if (signal?.aborted) throw new DOMException('Aborted', 'AbortError');
      const tokens = getTokens();
      if (tokens && tokens.accessExpiresAt - Date.now() < 30_000) await refreshTokens();
      let user: AssistantMessage | undefined;
      let reply: AssistantMessage | undefined;
      const requestStream = (stream = true) =>
        expoFetch(`${API_URL}/v1/tenant/assistant/threads/${id}/messages${stream ? '/stream' : ''}`, {
          method: 'POST',
          signal,
          credentials: 'omit',
          headers: {
            ...orgHeader(),
            Authorization: `Bearer ${getTokens()?.access ?? ''}`,
            Accept: stream ? 'text/event-stream' : 'application/json',
            'Content-Type': 'application/json',
            'Accept-Language': i18n.language,
          },
          body: JSON.stringify({ content, context }),
        });
      let response = await requestStream();
      if (response.status === 401 && getTokens()) {
        await response.body?.cancel().catch(() => undefined);
        if (await refreshTokens()) response = await requestStream();
      }
      // Only a server that does not support SSE can be retried synchronously.
      // Once accepted, replaying a POST could duplicate a saved message and its bill.
      if ([404, 405, 406, 415, 501].includes(response.status)) {
        await response.body?.cancel().catch(() => undefined);
        // Keep Stop effective on the synchronous fallback as well.
        const fallback = await requestStream(false);
        const payload = (await fallback.json()) as { data?: { message: AssistantMessage; reply: AssistantMessage } };
        if (!fallback.ok || !payload.data) throw toApiError(fallback.status, payload);
        return { thread: id, ...payload.data };
      }
      if (!response.ok || !response.body) throw toApiError(response.status, await response.json().catch(() => null));
      try {
        await readSSE(response.body as ReadableStream<Uint8Array>, (name, data) => {
          if (name === 'error') throw new ApiError(503, { code: (data as { code: string }).code });
          if (name === 'message.created') user = data as AssistantMessage;
          if (name === 'done') reply = (data as { reply: AssistantMessage }).reply;
          onEvent?.(name, data);
        });
      } finally {
        if (user) {
          for (let attempt = 0; attempt < 12; attempt++) {
            const data = await unwrap(
              api.GET('/v1/tenant/assistant/threads/{uuid}/messages', {
                params: { header: orgHeader(), path: { uuid: id } },
              }),
            ).catch(() => null);
            const index = data?.items.findIndex((m) => m.uuid === user?.uuid) ?? -1;
            if (data && index >= 0 && data.items.slice(index + 1).some((m) => m.role === 'assistant')) {
              qc.setQueryData(assistantKeys.messages(org, id), data.items);
              break;
            }
            await new Promise((resolve) => setTimeout(resolve, 150));
          }
        }
        void qc.invalidateQueries({ queryKey: assistantKeys.messages(org, id) });
      }
      if (!user || !reply) throw new Error('ASSISTANT_STREAM_INTERRUPTED');
      const r = { message: user, reply };
      return { thread: id, ...r };
    },
    onSuccess: (r) => {
      qc.setQueryData<AssistantMessage[]>(assistantKeys.messages(org, r.thread), (old) =>
        appendMessages(old, [r.message, r.reply]),
      );
      void qc.invalidateQueries({ queryKey: assistantKeys.threads(org) });
      void qc.invalidateQueries({ queryKey: assistantKeys.status(org) });
    },
  });
}

/** Confirms (runs) or rejects an action the assistant proposed. */
export function useDecideAction(thread: string) {
  const org = useOrgSlug();
  const qc = useQueryClient();
  const key = assistantKeys.messages(org, thread);
  return useMutation({
    mutationFn: async ({ action, decision }: { action: string; decision: 'confirm' | 'reject' }) => {
      const params = { header: orgHeader(), path: { uuid: thread, action_id: action } };
      if (decision === 'reject') {
        await unwrapVoid(api.POST('/v1/tenant/assistant/threads/{uuid}/actions/{action_id}/reject', { params }));
        return null;
      }
      return unwrap(api.POST('/v1/tenant/assistant/threads/{uuid}/actions/{action_id}/confirm', { params }));
    },
    onSuccess: (result, { action, decision }) => {
      qc.setQueryData<AssistantMessage[]>(key, (old) => {
        const next = withActionStatus(old, action, decision === 'reject' ? 'rejected' : 'confirmed');
        return result ? appendMessages(next, [result]) : next;
      });
    },
    // The server has the final status (done / failed).
    onSettled: () => void qc.invalidateQueries({ queryKey: key }),
  });
}

/** Hands the conversation to ClickFix's own support team. */
export function useEscalate(thread: string | null) {
  return useMutation({
    mutationFn: (note: string) =>
      unwrap(
        api.POST('/v1/tenant/assistant/threads/{uuid}/escalate', {
          params: { header: orgHeader(), path: { uuid: thread! } },
          body: { note: note.trim() },
        }),
      ),
  });
}
