import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useEffect, useRef } from 'react';
import { AppState } from 'react-native';

import { api, orgHeader, unwrap } from '@/core/api/client';
import type { components } from '@/core/api/schema';
import { P, useCan } from '@/core/auth/permissions';

import { useInboxScope } from './api';
import { inboxKeys } from './cache';

/**
 * Everything around one conversation beyond reading and replying (CF-79):
 * tags, team, reminders, links / merge / split, side conversations, who is
 * viewing, macros and engineering issues. Same endpoints as the panel's
 * conversation header and context panel.
 */
export type Reminder = components['schemas']['Reminder'];
export type ConversationViewer = components['schemas']['ConversationViewer'];
export type LinkedConversation = components['schemas']['LinkedConversation'];
export type SideConversation = components['schemas']['SideConversation'];
export type Macro = components['schemas']['Macro'];
export type EngineeringIssue = components['schemas']['EngineeringIssue'];

const keys = {
  reminders: (org: string, uuid: string) => [...inboxKeys.all(org), 'reminders', uuid] as const,
  links: (org: string, uuid: string) => [...inboxKeys.all(org), 'links', uuid] as const,
  sides: (org: string, uuid: string) => [...inboxKeys.all(org), 'sides', uuid] as const,
  side: (org: string, uuid: string) => [...inboxKeys.all(org), 'side', uuid] as const,
  viewers: (org: string, uuid: string) => [...inboxKeys.all(org), 'viewers', uuid] as const,
  macros: (org: string) => [...inboxKeys.all(org), 'macros'] as const,
  eng: (org: string, uuid: string) => [...inboxKeys.all(org), 'engineering', uuid] as const,
};

/** Refreshes the conversation itself, its timeline and the lists after a change. */
function useRefresh(uuid: string) {
  const { org } = useInboxScope();
  const qc = useQueryClient();
  return () => {
    void qc.invalidateQueries({ queryKey: inboxKeys.detail(org, uuid) });
    void qc.invalidateQueries({ queryKey: inboxKeys.messages(org, uuid) });
    void qc.invalidateQueries({ queryKey: inboxKeys.lists(org) });
    void qc.invalidateQueries({ queryKey: inboxKeys.dashboardSla(org) });
  };
}

export function useAddTag(uuid: string) {
  const refresh = useRefresh(uuid);
  return useMutation({
    mutationFn: (tag: { uuid?: string; name?: string }) =>
      unwrap(
        api.POST('/v1/tenant/conversations/{uuid}/tags', {
          params: { header: orgHeader(), path: { uuid } },
          body: tag,
        }),
      ),
    onSettled: refresh,
  });
}

export function useRemoveTag(uuid: string) {
  const refresh = useRefresh(uuid);
  return useMutation({
    mutationFn: (tag: string) =>
      unwrap(
        api.DELETE('/v1/tenant/conversations/{uuid}/tags/{tag}', {
          params: { header: orgHeader(), path: { uuid, tag } },
        }),
      ),
    onSettled: refresh,
  });
}

export function useReminders(uuid: string) {
  const { org } = useInboxScope();
  return useQuery({
    queryKey: keys.reminders(org, uuid),
    enabled: !!org && !!uuid,
    staleTime: 30_000,
    queryFn: () =>
      unwrap(api.GET('/v1/tenant/conversations/{uuid}/reminders', { params: { header: orgHeader(), path: { uuid } } })),
  });
}

export function useAddReminder(uuid: string) {
  const { org } = useInboxScope();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: components['schemas']['ReminderInput']) =>
      unwrap(
        api.POST('/v1/tenant/conversations/{uuid}/reminders', {
          params: { header: orgHeader(), path: { uuid } },
          body,
        }),
      ),
    onSettled: () => void qc.invalidateQueries({ queryKey: keys.reminders(org, uuid) }),
  });
}

export function useCancelReminder(uuid: string) {
  const { org } = useInboxScope();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (reminder: string) =>
      unwrap(
        api.DELETE('/v1/tenant/conversation-reminders/{uuid}', {
          params: { header: orgHeader(), path: { uuid: reminder } },
        }),
      ),
    onSettled: () => void qc.invalidateQueries({ queryKey: keys.reminders(org, uuid) }),
  });
}

export function useLinks(uuid: string) {
  const { org } = useInboxScope();
  return useQuery({
    queryKey: keys.links(org, uuid),
    enabled: !!org && !!uuid,
    staleTime: 60_000,
    queryFn: () =>
      unwrap(api.GET('/v1/tenant/conversations/{uuid}/links', { params: { header: orgHeader(), path: { uuid } } })),
  });
}

export function useLinkConversation(uuid: string) {
  const { org } = useInboxScope();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: components['schemas']['ConversationLinkRequest']) =>
      unwrap(
        api.POST('/v1/tenant/conversations/{uuid}/links', { params: { header: orgHeader(), path: { uuid } }, body }),
      ),
    onSuccess: (links) => qc.setQueryData(keys.links(org, uuid), links),
  });
}

/** Merges `source` into this conversation (the source closes and points here). */
export function useMerge(uuid: string) {
  const { org } = useInboxScope();
  const qc = useQueryClient();
  const refresh = useRefresh(uuid);
  return useMutation({
    mutationFn: (source: string) =>
      unwrap(
        api.POST('/v1/tenant/conversations/{uuid}/merge', {
          params: { header: orgHeader(), path: { uuid } },
          body: { source_uuid: source },
        }),
      ),
    onSettled: () => {
      refresh();
      void qc.invalidateQueries({ queryKey: keys.links(org, uuid) });
    },
  });
}

/** Moves a message (and, with `following`, the ones after it) into a new conversation. */
export function useSplit(uuid: string) {
  const refresh = useRefresh(uuid);
  return useMutation({
    mutationFn: (body: components['schemas']['ConversationSplitRequest']) =>
      unwrap(
        api.POST('/v1/tenant/conversations/{uuid}/split', { params: { header: orgHeader(), path: { uuid } }, body }),
      ),
    onSettled: refresh,
  });
}

export function useSideConversations(uuid: string) {
  const { org } = useInboxScope();
  return useQuery({
    queryKey: keys.sides(org, uuid),
    enabled: !!org && !!uuid,
    staleTime: 30_000,
    queryFn: () =>
      unwrap(
        api.GET('/v1/tenant/conversations/{uuid}/side-conversations', {
          params: { header: orgHeader(), path: { uuid } },
        }),
      ),
  });
}

export function useCreateSideConversation(uuid: string) {
  const { org } = useInboxScope();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: components['schemas']['SideConversationCreate']) =>
      unwrap(
        api.POST('/v1/tenant/conversations/{uuid}/side-conversations', {
          params: { header: orgHeader(), path: { uuid } },
          body,
        }),
      ),
    onSettled: () => void qc.invalidateQueries({ queryKey: keys.sides(org, uuid) }),
  });
}

export function useSideConversation(side: string) {
  const { org } = useInboxScope();
  return useQuery({
    queryKey: keys.side(org, side),
    enabled: !!org && !!side,
    staleTime: 15_000,
    refetchInterval: 30_000,
    queryFn: () =>
      unwrap(
        api.GET('/v1/tenant/side-conversations/{uuid}', { params: { header: orgHeader(), path: { uuid: side } } }),
      ),
  });
}

export function useReplySide(side: string) {
  const { org } = useInboxScope();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: string) =>
      unwrap(
        api.POST('/v1/tenant/side-conversations/{uuid}/messages', {
          params: { header: orgHeader(), path: { uuid: side } },
          body: { body },
        }),
      ),
    onSuccess: (s) => qc.setQueryData(keys.side(org, side), s),
  });
}

export function useSetSideStatus(side: string) {
  const { org } = useInboxScope();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (status: 'open' | 'closed') =>
      unwrap(
        api.PATCH('/v1/tenant/side-conversations/{uuid}', {
          params: { header: orgHeader(), path: { uuid: side } },
          body: { status },
        }),
      ),
    onSuccess: (s) => {
      qc.setQueryData(keys.side(org, side), s);
      void qc.invalidateQueries({ queryKey: keys.sides(org, s.conversation_uuid) });
    },
  });
}

/**
 * Collision detection (the panel's viewers bar): heartbeats "I am here" (and
 * whether I am typing) every 15 s while the screen is open and in the
 * foreground, leaves on close, and returns the other people viewing.
 */
export function useViewers(uuid: string, typing: boolean) {
  const { org } = useInboxScope();
  const qc = useQueryClient();
  const typingRef = useRef(typing);
  const beat = (t: boolean) =>
    unwrap(
      api.POST('/v1/tenant/conversations/{uuid}/presence', {
        params: { header: orgHeader(), path: { uuid } },
        body: { typing: t },
      }),
    )
      .then((r) => qc.setQueryData(keys.viewers(org, uuid), r.viewers))
      .catch(() => undefined);

  useEffect(() => {
    if (!org || !uuid) return;
    void beat(false);
    const id = setInterval(() => {
      if (AppState.currentState === 'active') void beat(typingRef.current);
    }, 15_000);
    return () => {
      clearInterval(id);
      void unwrap(
        api.DELETE('/v1/tenant/conversations/{uuid}/presence', { params: { header: orgHeader(), path: { uuid } } }),
      ).catch(() => undefined);
    };
    // beat is stable for the screen's lifetime.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [org, uuid]);

  // Typing changes are told at once (throttled by the composer's own state changes).
  useEffect(() => {
    if (typingRef.current === typing) return;
    typingRef.current = typing;
    if (org && uuid) void beat(typing);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [typing]);

  return useQuery<ConversationViewer[]>({
    queryKey: keys.viewers(org, uuid),
    enabled: false,
    initialData: [],
    queryFn: () => [],
  }).data;
}

export function useMacros(enabled: boolean) {
  const { org, canCanned } = useInboxScope();
  return useQuery({
    queryKey: keys.macros(org),
    enabled: !!org && enabled && canCanned,
    staleTime: 5 * 60_000,
    queryFn: () => unwrap(api.GET('/v1/tenant/macros', { params: { header: orgHeader() } })),
  });
}

export function useApplyMacro(uuid: string) {
  const refresh = useRefresh(uuid);
  return useMutation({
    mutationFn: (macro: string) =>
      unwrap(
        api.POST('/v1/tenant/conversations/{uuid}/macros/{macro}', {
          params: { header: orgHeader(), path: { uuid, macro } },
        }),
      ),
    onSettled: refresh,
  });
}

/** Engineering issues the API suggests for this conversation, and its escalations. */
export function useConversationEngineering(uuid: string) {
  const { org } = useInboxScope();
  const can = useCan();
  const enabled = !!org && !!uuid && can(P.ticketsRead);
  const suggestions = useQuery({
    queryKey: [...keys.eng(org, uuid), 'suggestions'] as const,
    enabled,
    staleTime: 60_000,
    retry: false,
    queryFn: () =>
      unwrap(
        api.GET('/v1/tenant/engineering/conversations/{conversation}/suggestions', {
          params: { header: orgHeader(), path: { conversation: uuid } },
        }),
      ),
  });
  const escalations = useQuery({
    queryKey: [...keys.eng(org, uuid), 'escalations'] as const,
    enabled,
    staleTime: 60_000,
    retry: false,
    queryFn: () =>
      unwrap(
        api.GET('/v1/tenant/engineering/conversations/{conversation}/escalations', {
          params: { header: orgHeader(), path: { conversation: uuid } },
        }),
      ),
  });
  return { enabled, suggestions, escalations };
}
