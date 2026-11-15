"use client";

import Link from "next/link";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { useCallback, useRef, useState } from "react";
import { ArrowLeft, Lock, SearchX } from "lucide-react";

import { EmptyState } from "@/components/common/empty-state";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { routes } from "@/config/routes";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";

import { useContactOverview } from "../../hooks/use-contact-profile";
import { useContactsScope } from "../../hooks/use-contacts";
import type { ContactOverview } from "../../services/profile.service";
import { EditContactSheet } from "./contact-dialogs";
import { ContactHeader } from "./contact-header";
import {
  ContactAISummaryCard,
  ContactStats,
  FilesCard,
  IdentitiesCard,
  StatusNoticesCard,
} from "./contact-overview-cards";
import {
  AppointmentsSection,
  CompanySection,
  ComplianceSection,
  ConversationsSection,
  FeedbackSection,
  FieldsSection,
} from "./contact-sections";
import {
  ContactTimeline,
  type ContactTimelineHandle,
} from "./contact-timeline";
import { isTab, type ContactTab } from "./lib";

/** Everything known about one contact on one page (CF-221). */
export function ContactPage({ uuid }: { uuid: string }) {
  const { t } = useLocale();
  const scope = useContactsScope();
  const overview = useContactOverview(uuid);

  if (!scope.canRead) {
    return (
      <EmptyState
        icon={<Lock className="size-6" />}
        title={t("contacts.no_permission")}
      />
    );
  }
  if (overview.isLoading) return <ContactPageSkeleton />;
  if (!overview.data) {
    const notFound =
      isApiError(overview.error) && overview.error.status === 404;
    return (
      <div className="mx-auto w-full max-w-7xl">
        <BackLink />
        <EmptyState
          icon={<SearchX className="size-6" />}
          title={
            notFound
              ? t("contacts.profile.not_found")
              : t("contacts.profile.load_failed")
          }
          description={
            notFound ? t("contacts.profile.not_found_hint") : undefined
          }
          action={
            notFound ? (
              <Button size="sm" variant="outline" asChild>
                <Link href={routes.cms.contacts.root}>
                  {t("contacts.title")}
                </Link>
              </Button>
            ) : (
              <Button
                size="sm"
                variant="outline"
                onClick={() => void overview.refetch()}
              >
                {t("contacts.profile.retry")}
              </Button>
            )
          }
        />
      </div>
    );
  }
  return <ContactPageBody data={overview.data} />;
}

function BackLink() {
  const { t } = useLocale();
  return (
    <Link
      href={routes.cms.contacts.root}
      className="text-muted-foreground hover:text-foreground focus-visible:ring-ring mb-1 flex w-fit items-center gap-1 rounded text-sm outline-none focus-visible:ring-2"
    >
      <ArrowLeft className="size-4" />
      {t("contacts.title")}
    </Link>
  );
}

function ContactPageBody({ data }: { data: ContactOverview }) {
  const { t } = useLocale();
  const scope = useContactsScope();
  const router = useRouter();
  const pathname = usePathname();
  const params = useSearchParams();
  const timeline = useRef<ContactTimelineHandle>(null);
  const [editing, setEditing] = useState(false);
  const { sections } = data;

  const visible: { id: ContactTab; label: string; count?: number }[] = [
    { id: "activity", label: t("contacts.profile.tab.activity") },
    ...(sections.conversations
      ? [
          {
            id: "conversations" as const,
            label: t("contacts.profile.tab.conversations"),
            count:
              data.stats.open_conversations + data.stats.closed_conversations,
          },
        ]
      : []),
    { id: "company", label: t("contacts.profile.tab.company") },
    { id: "fields", label: t("contacts.profile.tab.fields") },
    ...(sections.appointments
      ? [
          {
            id: "appointments" as const,
            label: t("contacts.profile.tab.appointments"),
            count: data.appointments.length,
          },
        ]
      : []),
    ...(sections.feedback
      ? [
          {
            id: "feedback" as const,
            label: t("contacts.profile.tab.feedback"),
            count: data.feedback.length + data.forum_posts.length,
          },
        ]
      : []),
    ...(sections.compliance
      ? [
          {
            id: "compliance" as const,
            label: t("contacts.profile.tab.compliance"),
            count: data.privacy_requests.length,
          },
        ]
      : []),
  ];
  const raw = params.get("tab");
  const tab: ContactTab =
    isTab(raw) && visible.some((v) => v.id === raw) ? raw : "activity";
  const setTab = useCallback(
    (next: string) => {
      const q = new URLSearchParams(params.toString());
      if (next === "activity") q.delete("tab");
      else q.set("tab", next);
      const qs = q.toString();
      router.replace(qs ? `${pathname}?${qs}` : pathname, { scroll: false });
    },
    [params, pathname, router],
  );
  const addNote = () => {
    setTab("activity");
    // The composer mounts with the activity tab.
    window.setTimeout(() => timeline.current?.focusComposer(), 50);
  };

  return (
    <div className="mx-auto flex w-full max-w-7xl flex-col gap-4">
      <BackLink />
      <ContactHeader data={data} onAddNote={addNote} />
      <ContactStats data={data} />

      <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_340px]">
        <Tabs value={tab} onValueChange={setTab} className="min-w-0 gap-4">
          <div className="-mx-4 overflow-x-auto px-4 pb-1 sm:mx-0 sm:px-0">
            <TabsList className="w-max">
              {visible.map((v) => (
                <TabsTrigger key={v.id} value={v.id} className="gap-1.5 px-3">
                  {v.label}
                  {v.count ? (
                    <span className="bg-background/80 text-muted-foreground rounded-full px-1.5 text-[10px] tabular-nums">
                      {v.count}
                    </span>
                  ) : null}
                </TabsTrigger>
              ))}
            </TabsList>
          </div>
          <TabsContent value="activity">
            <section className="bg-card rounded-2xl border p-4">
              <ContactTimeline
                ref={timeline}
                uuid={data.contact.uuid}
                kinds={data.timeline_kinds}
              />
            </section>
          </TabsContent>
          {sections.conversations && (
            <TabsContent value="conversations">
              <ConversationsSection data={data} />
            </TabsContent>
          )}
          <TabsContent value="company">
            <CompanySection
              data={data}
              canWrite={scope.canWrite}
              onEdit={() => setEditing(true)}
            />
          </TabsContent>
          <TabsContent value="fields">
            <FieldsSection data={data} />
          </TabsContent>
          {sections.appointments && (
            <TabsContent value="appointments">
              <AppointmentsSection data={data} />
            </TabsContent>
          )}
          {sections.feedback && (
            <TabsContent value="feedback">
              <FeedbackSection data={data} />
            </TabsContent>
          )}
          {sections.compliance && (
            <TabsContent value="compliance">
              <ComplianceSection data={data} />
            </TabsContent>
          )}
        </Tabs>

        <aside
          className="flex flex-col gap-4"
          aria-label={t("contacts.profile.aside")}
        >
          <ContactAISummaryCard data={data} />
          <IdentitiesCard data={data} />
          <FilesCard data={data} />
          <StatusNoticesCard data={data} />
        </aside>
      </div>
      <EditContactSheet
        contact={data.contact}
        open={editing}
        onOpenChange={setEditing}
      />
    </div>
  );
}

/** Loading skeleton in the page's shape. */
export function ContactPageSkeleton() {
  return (
    <div
      className="mx-auto flex w-full max-w-7xl flex-col gap-4"
      aria-busy="true"
    >
      <Skeleton className="h-5 w-24" />
      <div className="bg-card flex gap-4 rounded-2xl border p-5">
        <Skeleton className="size-16 shrink-0 rounded-2xl" />
        <div className="grid flex-1 gap-2">
          <Skeleton className="h-7 w-56" />
          <Skeleton className="h-4 w-72 max-w-full" />
          <div className="flex gap-2">
            <Skeleton className="h-6 w-40 rounded-full" />
            <Skeleton className="h-6 w-32 rounded-full" />
          </div>
        </div>
      </div>
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 xl:grid-cols-6">
        {[0, 1, 2, 3, 4, 5].map((i) => (
          <Skeleton key={i} className="h-[84px] rounded-xl" />
        ))}
      </div>
      <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_340px]">
        <div className="grid gap-4">
          <Skeleton className="h-9 w-full max-w-lg rounded-lg" />
          <Skeleton className="h-96 rounded-2xl" />
        </div>
        <div className="grid content-start gap-4">
          <Skeleton className="h-48 rounded-2xl" />
          <Skeleton className="h-32 rounded-2xl" />
        </div>
      </div>
    </div>
  );
}
