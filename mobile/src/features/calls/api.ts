import { useInfiniteQuery, useQuery } from '@tanstack/react-query';

import { api, orgHeader, unwrap } from '@/core/api/client';
import { isApiError } from '@/core/api/errors';
import { orgKey, useOrgSlug } from '@/core/api/org';
import type { components } from '@/core/api/schema';
import { P, useCan } from '@/core/auth/permissions';
import { nextOffset, PAGE_SIZE } from '@/ui/paged-list';

/**
 * Call log of a voice channel (CF-169): the endpoint the panel's page reads
 * (`frontend/src/features/channels/services/channels.service.ts`,
 * `voiceCalls`), with `tenant.channels.read`. The API pages by limit/offset
 * and takes no filter.
 */
export type VoiceCall = components['schemas']['VoiceCall'];

export function useCallScope() {
  const org = useOrgSlug();
  const can = useCan();
  return { org, canRead: can(P.channelsRead), canTranscript: can(P.conversationsRead) };
}

export function useVoiceCalls(channel: string) {
  const { org, canRead } = useCallScope();
  return useInfiniteQuery({
    queryKey: orgKey(org, 'admin', 'voice-calls', channel),
    enabled: !!org && canRead && !!channel,
    initialPageParam: 0,
    staleTime: 30_000,
    queryFn: ({ pageParam }) =>
      unwrap(
        api.GET('/v1/tenant/channels/{uuid}/voice/calls', {
          params: { header: orgHeader(), path: { uuid: channel }, query: { limit: PAGE_SIZE, offset: pageParam } },
        }),
      ),
    getNextPageParam: nextOffset,
  });
}

/**
 * The retained transcript of a recorded call (`tenant.conversations.read`
 * and the conversation's team visibility). The API answers 404 once it was
 * erased or expired, which is not worth a retry.
 */
export function useCallTranscript(uuid: string | null | undefined) {
  const { org, canTranscript } = useCallScope();
  return useQuery({
    queryKey: orgKey(org, 'admin', 'call-transcript', uuid ?? ''),
    enabled: !!org && canTranscript && !!uuid,
    staleTime: 60_000,
    retry: (count, e) => !(isApiError(e) && [403, 404].includes(e.status)) && count < 2,
    queryFn: () =>
      unwrap(
        api.GET('/v1/tenant/recording-transcripts/{uuid}', {
          params: { header: orgHeader(), path: { uuid: uuid as string } },
        }),
      ),
  });
}
