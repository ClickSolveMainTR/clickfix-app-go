"use client";

import {
  useMutation,
  useQueries,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";

import { permissions } from "@/config/permissions";
import {
  engineeringService,
  type Issue,
  type IssueCreate,
  type IssuePatch,
  type ListIssuesParams,
} from "@/features/engineering/services/engineering.service";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

/** Query keys carry the organization slug. */
export const engineeringKeys = {
  all: (slug: string) => ["tenant", slug, "engineering"] as const,
  list: (slug: string, params: Record<string, unknown>) =>
    [...engineeringKeys.all(slug), "list", params] as const,
  issue: (slug: string, uuid: string) =>
    [...engineeringKeys.all(slug), "issue", uuid] as const,
  links: (slug: string, uuid: string) =>
    [...engineeringKeys.issue(slug, uuid), "links"] as const,
  comments: (slug: string, uuid: string) =>
    [...engineeringKeys.issue(slug, uuid), "comments"] as const,
  conversation: (slug: string, uuid: string) =>
    [...engineeringKeys.all(slug), "conversation", uuid] as const,
};

export function useEngineeringScope() {
  const { slug } = useTenant();
  const { can } = usePermission();
  return {
    slug,
    canRead: can(permissions.tenant.tickets.read),
    canWrite: can(permissions.tenant.tickets.write),
    canReadConversations: can(permissions.tenant.conversations.read),
    /** The AI summary of a new request needs reply access. */
    canSummarize: can(permissions.tenant.conversations.reply),
  };
}

export function useIssues(params: ListIssuesParams, enabled = true) {
  const { slug, canRead } = useEngineeringScope();
  return useQuery({
    queryKey: engineeringKeys.list(slug, params),
    queryFn: () => engineeringService.list(slug, params),
    enabled: enabled && Boolean(slug) && canRead,
    placeholderData: (previous) => previous,
    staleTime: 30_000,
  });
}

export function useIssue(uuid: string) {
  const { slug, canRead } = useEngineeringScope();
  return useQuery({
    queryKey: engineeringKeys.issue(slug, uuid),
    queryFn: () => engineeringService.get(slug, uuid),
    enabled: Boolean(slug && uuid) && canRead,
    staleTime: 30_000,
  });
}

export function useIssueConversations(uuid: string) {
  const { slug, canRead, canReadConversations } = useEngineeringScope();
  return useQuery({
    queryKey: engineeringKeys.links(slug, uuid),
    queryFn: () => engineeringService.conversations(slug, uuid),
    enabled: Boolean(slug && uuid) && canRead && canReadConversations,
    staleTime: 30_000,
  });
}

export function useIssueComments(uuid: string) {
  const { slug, canRead } = useEngineeringScope();
  return useQuery({
    queryKey: engineeringKeys.comments(slug, uuid),
    queryFn: () => engineeringService.comments(slug, uuid),
    enabled: Boolean(slug && uuid) && canRead,
    staleTime: 30_000,
  });
}

export function useCreateIssue() {
  const { slug } = useEngineeringScope();
  const { t } = useLocale();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (body: IssueCreate) => engineeringService.create(slug, body),
    onSuccess: () => appToast.success(t("engineering.toast.created")),
    onSettled: () =>
      void queryClient.invalidateQueries({
        queryKey: engineeringKeys.all(slug),
      }),
  });
}

export function useLinkIssue() {
  const { slug } = useEngineeringScope();
  const { t } = useLocale();
  const queryClient = useQueryClient();
  return useMutation({
    // Linking is an upsert that also rewrites `notify_customer`, so an
    // existing link (made with the request, notifying the customer) is kept.
    mutationFn: async (v: {
      issue: string;
      conversation: string;
      notifyCustomer: boolean;
    }) => {
      const links = await queryClient.fetchQuery({
        queryKey: engineeringKeys.links(slug, v.issue),
        queryFn: () => engineeringService.conversations(slug, v.issue),
        staleTime: 0,
      });
      if (links.items.some((l) => l.conversation_uuid === v.conversation)) {
        return { alreadyLinked: true };
      }
      await engineeringService.link(
        slug,
        v.issue,
        v.conversation,
        v.notifyCustomer,
      );
      return { alreadyLinked: false };
    },
    onSuccess: (result) =>
      result.alreadyLinked
        ? appToast.info(t("engineering.toast.already_linked"))
        : appToast.success(t("engineering.toast.linked")),
    onSettled: () =>
      void queryClient.invalidateQueries({
        queryKey: engineeringKeys.all(slug),
      }),
  });
}

export function useUpdateIssue(uuid: string) {
  const { slug } = useEngineeringScope();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (body: IssuePatch) =>
      engineeringService.update(slug, uuid, body),
    onSuccess: (issue: Issue) =>
      queryClient.setQueryData(engineeringKeys.issue(slug, uuid), issue),
    // A stale `revision` is rejected; refetching shows the current request.
    onSettled: () =>
      void queryClient.invalidateQueries({
        queryKey: engineeringKeys.all(slug),
      }),
  });
}

export function useAddIssueComment(uuid: string) {
  const { slug } = useEngineeringScope();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (body: string) =>
      engineeringService.addComment(slug, uuid, body),
    onSettled: () =>
      void queryClient.invalidateQueries({
        queryKey: engineeringKeys.comments(slug, uuid),
      }),
  });
}

/** Suggested requests and escalations of one conversation (inbox panel). */
export function useConversationEngineering(conversation: string) {
  const { slug, canRead, canReadConversations } = useEngineeringScope();
  const enabled =
    Boolean(slug && conversation) && canRead && canReadConversations;
  const suggestions = useQuery({
    queryKey: [
      ...engineeringKeys.conversation(slug, conversation),
      "suggestions",
    ],
    queryFn: () => engineeringService.suggestions(slug, conversation),
    enabled,
    staleTime: 60_000,
    retry: false,
  });
  const escalations = useQuery({
    queryKey: [
      ...engineeringKeys.conversation(slug, conversation),
      "escalations",
    ],
    queryFn: () => engineeringService.escalations(slug, conversation),
    enabled,
    staleTime: 60_000,
    retry: false,
  });
  // Which suggestions this conversation is already linked to.
  const suggested = suggestions.data?.items ?? [];
  const links = useQueries({
    queries: suggested.map((issue) => ({
      queryKey: engineeringKeys.links(slug, issue.uuid),
      queryFn: () => engineeringService.conversations(slug, issue.uuid, true),
      enabled,
      staleTime: 30_000,
      retry: false,
    })),
  });
  const linked = new Set<string>();
  /** Suggestions whose links are not known yet; they cannot be linked. */
  const unknown = new Set<string>();
  suggested.forEach((issue, index) => {
    const items = links[index]?.data?.items;
    if (!items) unknown.add(issue.uuid);
    else if (items.some((l) => l.conversation_uuid === conversation))
      linked.add(issue.uuid);
  });
  return { enabled, suggestions, escalations, linked, unknown };
}
