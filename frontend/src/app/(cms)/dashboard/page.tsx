"use client";

import { ArrowRight } from "lucide-react";
import Link from "next/link";

import { EmptyState } from "@/components/common/empty-state";
import { PageHeader } from "@/components/layout/page-header";
import { Button } from "@/components/ui/button";
import { routes } from "@/config/routes";
import { AppointmentsUpcomingWidget } from "@/features/calendar";
import { SetupChecklistCard } from "@/features/onboarding";
import { useActiveOrganization } from "@/features/organizations";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";

/**
 * Workspace home: greeting for the active organization, its set-up
 * checklist while it is open, or the way to open a first organization.
 */
export default function CmsHomePage() {
  const { t } = useLocale();
  const { user } = useAuth();
  const { active } = useActiveOrganization();
  const firstName = user?.name?.trim();

  return (
    <div className="mx-auto flex w-full max-w-5xl flex-col gap-6">
      <PageHeader
        eyebrow={active?.name ?? t("cms.home.eyebrow")}
        title={
          firstName
            ? t("cms.home.greeting", { name: firstName })
            : t("cms.home.title")
        }
        description={t("cms.home.subtitle")}
      />
      {active ? (
        <>
          <SetupChecklistCard />
          <AppointmentsUpcomingWidget />
          <EmptyState
            title={t("cms.home.empty_title")}
            description={t("cms.home.empty_description")}
          />
        </>
      ) : (
        <EmptyState
          title={t("cms.home.no_org_title")}
          description={t("cms.home.no_org_description")}
          action={
            <Button asChild>
              <Link href={routes.cms.onboarding}>
                {t("cms.home.no_org_action")}
                <ArrowRight />
              </Link>
            </Button>
          }
        />
      )}
    </div>
  );
}
