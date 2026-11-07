"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { channelsService } from "@/features/channels/services/channels.service";
import type {
  ChannelInstanceCreate,
  ChannelInstanceUpdate,
  WhatsAppTemplateCreate,
} from "@/features/channels/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/** Query keys carry the organization slug. */
export const channelsKeys = {
  all: (slug: string) => ["tenant", slug, "channels"] as const,
  list: (slug: string) => [...channelsKeys.all(slug), "list"] as const,
  types: (slug: string) => [...channelsKeys.all(slug), "types"] as const,
  domain: (slug: string, domain: string) =>
    [...channelsKeys.all(slug), "email-domain", domain] as const,
  templates: (slug: string, uuid: string) =>
    [...channelsKeys.all(slug), "templates", uuid] as const,
  signup: (slug: string) => [...channelsKeys.all(slug), "wa-signup"] as const,
  slack: (slug: string, uuid: string) =>
    [...channelsKeys.all(slug), "slack-conversations", uuid] as const,
  googleChat: (slug: string, uuid: string) =>
    [...channelsKeys.all(slug), "google-chat-spaces", uuid] as const,
  calls: (slug: string, uuid: string, limit: number, offset: number) =>
    [...channelsKeys.all(slug), "voice-calls", uuid, limit, offset] as const,
};

/** Instances refresh every minute so health and last-seen stay current. */
export function useChannels() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: channelsKeys.list(slug),
    queryFn: () => channelsService.list(slug).then((r) => r.items),
    refetchInterval: 60_000,
  });
}

export function useChannelTypes() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: channelsKeys.types(slug),
    queryFn: () => channelsService.types(slug).then((r) => r.items),
    staleTime: 10 * 60_000,
  });
}

/** Refetches every channels query of the active organization. */
export function useInvalidateChannels() {
  const { slug } = useTenant();
  const queryClient = useQueryClient();
  return () =>
    void queryClient.invalidateQueries({ queryKey: channelsKeys.all(slug) });
}

export function useCreateChannel() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateChannels();
  return useMutation({
    mutationFn: (body: ChannelInstanceCreate) =>
      channelsService.create(slug, body),
    onSuccess: () => {
      invalidate();
      appToast.success(t("channels.toast.connected"));
    },
  });
}

export function useUpdateChannel() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateChannels();
  return useMutation({
    mutationFn: ({
      uuid,
      body,
    }: {
      uuid: string;
      body: ChannelInstanceUpdate;
    }) => channelsService.update(slug, uuid, body),
    onSuccess: () => {
      invalidate();
      appToast.success(t("channels.toast.updated"));
    },
  });
}

export function useDeleteChannel() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateChannels();
  return useMutation({
    mutationFn: (uuid: string) => channelsService.remove(slug, uuid),
    onSuccess: () => {
      invalidate();
      appToast.success(t("channels.toast.disconnected"));
    },
  });
}

export function useCheckChannel() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateChannels();
  return useMutation({
    mutationFn: (uuid: string) => channelsService.check(slug, uuid),
    onSuccess: (inst) => {
      invalidate();
      if (inst.health.state === "error") {
        appToast.error(t("channels.toast.check_failed"));
      } else {
        appToast.success(t("channels.toast.check_ok"));
      }
    },
  });
}

export function useReconnectChannel() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateChannels();
  return useMutation({
    mutationFn: (uuid: string) => channelsService.reconnect(slug, uuid),
    onSuccess: () => {
      invalidate();
      appToast.success(t("channels.toast.reconnected"));
    },
  });
}

export function useEmailDomain(domain: string) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: channelsKeys.domain(slug, domain),
    queryFn: () => channelsService.emailDomainGet(slug, domain),
    enabled: domain !== "",
    retry: false,
  });
}

export function useRegisterEmailDomain() {
  const { slug } = useTenant();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (domain: string) =>
      channelsService.emailDomainCreate(slug, domain),
    onSuccess: (d) =>
      queryClient.setQueryData(channelsKeys.domain(slug, d.domain), d),
  });
}

export function useVerifyEmailDomain() {
  const { slug } = useTenant();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (domain: string) =>
      channelsService.emailDomainVerify(slug, domain),
    onSuccess: (d) =>
      queryClient.setQueryData(channelsKeys.domain(slug, d.domain), d),
  });
}

export function useWhatsAppSignupConfig(enabled: boolean) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: channelsKeys.signup(slug),
    queryFn: () => channelsService.whatsappSignupConfig(slug),
    enabled,
    retry: false,
    staleTime: 10 * 60_000,
  });
}

export function useWhatsAppTemplates(uuid: string) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: channelsKeys.templates(slug, uuid),
    queryFn: () =>
      channelsService.whatsappTemplates(slug, uuid).then((r) => r.items),
  });
}

export function useCreateWhatsAppTemplate(uuid: string) {
  const { slug } = useTenant();
  const { t } = useLocale();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (body: WhatsAppTemplateCreate) =>
      channelsService.whatsappTemplateCreate(slug, uuid, body),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: channelsKeys.templates(slug, uuid),
      });
      appToast.success(t("channels.templates.submitted"));
    },
  });
}

export function useDeleteWhatsAppTemplate(uuid: string) {
  const { slug } = useTenant();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (name: string) =>
      channelsService.whatsappTemplateDelete(slug, uuid, name),
    onSuccess: () =>
      void queryClient.invalidateQueries({
        queryKey: channelsKeys.templates(slug, uuid),
      }),
  });
}

/** Slack channels the bot was invited to (CF-119 allowlist picker). */
export function useSlackConversations(uuid: string) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: channelsKeys.slack(slug, uuid),
    queryFn: () =>
      channelsService.slackConversations(slug, uuid).then((r) => r.items),
    retry: false,
  });
}

/** Google Chat rooms the app was added to (CF-137 allowlist picker). */
export function useGoogleChatSpaces(uuid: string) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: channelsKeys.googleChat(slug, uuid),
    queryFn: () =>
      channelsService.googleChatSpaces(slug, uuid).then((r) => r.items),
    retry: false,
  });
}

/** One page of a voice channel's call log (CF-122). */
export function useVoiceCalls(uuid: string, limit: number, offset: number) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: channelsKeys.calls(slug, uuid, limit, offset),
    queryFn: () => channelsService.voiceCalls(slug, uuid, limit, offset),
    placeholderData: (prev) => prev,
    refetchInterval: 30_000,
  });
}
