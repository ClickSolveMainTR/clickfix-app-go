"use client";

import { useMutation, useQuery } from "@tanstack/react-query";
import { CheckCircle2 } from "lucide-react";

import { Loading } from "@/components/common/loading";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { AuthCard } from "@/features/auth/components/auth-card";
import { AuthShell } from "@/features/auth/components/auth-shell";
import { statusSubscriptionService } from "@/features/status-subscription/status-subscription.service";
import { useLocale } from "@/providers/locale-provider";

/**
 * Confirmation page of a status page e-mail link: "confirm my
 * subscription" or "unsubscribe". Nothing changes until the button is
 * pressed.
 */
export function StatusSubscriptionPage({
  slug,
  token,
}: {
  slug: string;
  token: string;
}) {
  const { t } = useLocale();
  const preview = useQuery({
    queryKey: ["status-subscription", slug, token],
    queryFn: () => statusSubscriptionService.preview(slug, token),
    retry: false,
  });
  const apply = useMutation({
    mutationFn: () => statusSubscriptionService.apply(slug, token),
  });

  let body: React.ReactNode;
  if (preview.isLoading) {
    body = <Loading label={t("common.loading")} />;
  } else if (preview.isError || !preview.data) {
    body = (
      <AuthCard
        title={t("oncall.status_subscription.invalid_title")}
        description={t("oncall.status_subscription.invalid_body")}
      >
        {null}
      </AuthCard>
    );
  } else {
    const unsubscribe = preview.data.action === "unsubscribe";
    const key = unsubscribe ? "unsubscribe" : "confirm";
    const page = preview.data.page_title;
    body = (
      <AuthCard
        title={t(`oncall.status_subscription.${key}_title`)}
        description={t(`oncall.status_subscription.${key}_body`, { page })}
      >
        {apply.isSuccess ? (
          <Alert>
            <CheckCircle2 className="size-4" aria-hidden />
            <AlertTitle>
              {t(`oncall.status_subscription.${key}_done_title`)}
            </AlertTitle>
            <AlertDescription>
              {t(`oncall.status_subscription.${key}_done_body`, { page })}
            </AlertDescription>
          </Alert>
        ) : (
          <div className="space-y-4">
            {apply.isError ? (
              <Alert variant="destructive">
                <AlertDescription>
                  {t("oncall.status_subscription.failed")}
                </AlertDescription>
              </Alert>
            ) : null}
            <Button
              className="w-full"
              variant={unsubscribe ? "destructive" : "default"}
              disabled={apply.isPending}
              onClick={() => apply.mutate()}
            >
              {t(`oncall.status_subscription.${key}_action`)}
            </Button>
          </div>
        )}
      </AuthCard>
    );
  }

  return <AuthShell pitch={false}>{body}</AuthShell>;
}
