import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { api, orgHeader, unwrap } from '@/core/api/client';
import { isApiError } from '@/core/api/errors';
import type { components } from '@/core/api/schema';
import { P, useCan } from '@/core/auth/permissions';
import { useSession } from '@/core/auth/session';
import { i18n } from '@/core/i18n';

/**
 * Video rooms (CF-139), the same endpoints as the panel's
 * `features/video/services/video.service.ts`. Members join with their session;
 * customers join from a signed `/r/{code}` link without one.
 */
export type VideoJoinGrant = components['schemas']['VideoJoinGrant'];
export type VideoRoom = components['schemas']['VideoRoom'];
export type VideoParticipant = components['schemas']['VideoParticipant'];
export type VideoRoomPublic = components['schemas']['VideoRoomPublic'];

export function useVideoScope() {
  const org = useSession((s) => s.organization);
  const can = useCan();
  return { org: org?.slug ?? '', canUse: can(P.videoUse) };
}

/** Localized text for a failed video call (`video:errors.VIDEO_*`). */
export function videoError(e: unknown) {
  const code = isApiError(e) && e.code.startsWith('VIDEO_') ? e.code : 'UNKNOWN';
  return i18n.t(`video:errors.${code}`);
}

const keys = {
  participants: (org: string, room: string) => ['video', org, 'participants', room] as const,
};

/** The organization's video policy; `require_pin` decides whether new rooms need a PIN. */
export function useVideoAvailability() {
  const { org, canUse } = useVideoScope();
  return useQuery({
    queryKey: ['video', org, 'availability'] as const,
    enabled: !!org && canUse,
    staleTime: 60_000,
    queryFn: () =>
      unwrap<components['schemas']['VideoAvailability']>(
        api.GET('/v1/tenant/video/availability', { params: { header: orgHeader() } }),
      ),
  });
}

/** Room PINs are 4-12 digits, like the panel's create dialog. */
export const cleanPin = (v: string) => v.replace(/\D/g, '').slice(0, 12);

/** Opens a room linked to the conversation (inbox header action). */
export function useStartConversationCall() {
  return useMutation({
    mutationFn: (v: { conversation: string; name: string; pin?: string }) =>
      unwrap<VideoRoom>(
        api.POST('/v1/tenant/video/rooms', {
          params: { header: orgHeader() },
          body: { name: v.name, conversation_uuid: v.conversation, pin: v.pin || undefined },
        }),
      ),
  });
}

export function joinAsMember(room: string) {
  return unwrap<VideoJoinGrant>(
    api.POST('/v1/tenant/video/rooms/{uuid}/join', { params: { header: orgHeader(), path: { uuid: room } } }),
  );
}

export function endRoom(room: string) {
  return unwrap(
    api.POST('/v1/tenant/video/rooms/{uuid}/end', { params: { header: orgHeader(), path: { uuid: room } } }),
  );
}

/** Lobby of a room the member is in (polled while the call is open). */
export function useLobby(room: string, enabled: boolean) {
  const { org } = useVideoScope();
  return useQuery({
    queryKey: keys.participants(org, room),
    enabled,
    refetchInterval: 3_000,
    queryFn: () =>
      unwrap<{ items: VideoParticipant[] }>(
        api.GET('/v1/tenant/video/rooms/{uuid}/participants', {
          params: { header: orgHeader(), path: { uuid: room } },
        }),
      ),
  });
}

export function useLobbyAction(room: string) {
  const { org } = useVideoScope();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (v: { participant: string; action: 'admit' | 'deny' }) => {
      const params = { header: orgHeader(), path: { uuid: room, participant: v.participant } };
      return unwrap(
        v.action === 'admit'
          ? api.POST('/v1/tenant/video/rooms/{uuid}/participants/{participant}/admit', { params })
          : api.POST('/v1/tenant/video/rooms/{uuid}/participants/{participant}/deny', { params }),
      );
    },
    onSettled: () => qc.invalidateQueries({ queryKey: keys.participants(org, room) }),
  });
}

export const publicVideo = {
  room: (code: string) =>
    unwrap<VideoRoomPublic>(api.GET('/v1/public/video/rooms/{code}', { params: { path: { code } } })),
  join: (code: string, body: { name: string; email?: string; pin?: string }) =>
    unwrap<VideoJoinGrant>(api.POST('/v1/public/video/rooms/{code}/join', { params: { path: { code } }, body })),
  ticket: (ticket: string) => unwrap<VideoJoinGrant>(api.POST('/v1/public/video/ticket', { body: { ticket } })),
};
