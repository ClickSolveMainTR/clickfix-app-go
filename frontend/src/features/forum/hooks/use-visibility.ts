"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

import { forumKeys } from "../lib/keys";
import {
  cancel,
  confirm,
  idle,
  request as stage,
  type TakedownRequest,
  type TakedownState,
  visibilityProblem,
} from "../lib/takedown";
import { forumService } from "../services/forum.service";

/**
 * Taking a post down and publishing it again (CF-182). `ask` stages a
 * request for the confirmation dialog; `confirmPending` sends it. Whatever
 * the answer, the forum is read again (a refusal means the screen was old).
 */
export function useVisibility(
  org: string,
  onDone?: (r: TakedownRequest) => void,
) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const [state, setState] = useState<TakedownState>(idle);
  const invalidate = () =>
    qc.invalidateQueries({ queryKey: forumKeys.all(org) });
  const mutation = useMutation({
    mutationFn: (r: TakedownRequest & { reason: string }) =>
      r.action === "unpublish"
        ? forumService.unpublish(r.uuid, r.reason)
        : forumService.republish(r.uuid),
    onSuccess: async (_d, r) => {
      await invalidate();
      appToast.success(
        t(
          r.action === "unpublish"
            ? "forum.toast.unpublished"
            : "forum.toast.republished",
        ),
      );
      onDone?.(r);
    },
    onError: async (error, r) => {
      const known = visibilityProblem(
        isApiError(error) ? error.status : null,
        r.action,
      );
      if (!known) {
        appToast.error(
          isApiError(error) ? error.message : t("forum.toast.failed"),
        );
        return;
      }
      await invalidate();
      appToast.warning(t(known));
    },
  });
  return {
    pending: state.pending,
    isPending: mutation.isPending,
    ask: (r: TakedownRequest) => setState((s) => stage(s, r)),
    cancel: () => setState((s) => cancel(s)),
    confirmPending: (reason: string) => {
      const { send, state: next } = confirm(state, reason);
      setState(next);
      if (send) mutation.mutate(send);
    },
  };
}
