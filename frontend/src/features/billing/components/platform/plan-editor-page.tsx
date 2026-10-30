"use client";

import { Archive, FilePlus2, Rocket, Save, Users } from "lucide-react";
import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import { EntityPage, EntitySectionCard } from "@/components/entity";
import { AppForm } from "@/components/forms";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  createPlanSchema,
  PlanFields,
  type PlanFormValues,
} from "@/features/billing/components/platform/plans-panel";
import { draftFromVersion } from "@/features/billing/components/platform/catalog/catalog-lib";
import { MigrateSubscribersDialog } from "@/features/billing/components/platform/catalog/migrate-subscribers-dialog";
import { VersionEditor } from "@/features/billing/components/platform/version-editor";
import {
  useArchivePlan,
  useBillingFeatures,
  usePlan,
  usePublishVersion,
  useSaveVersion,
  useUpdatePlan,
} from "@/features/billing/hooks/use-platform-billing";
import { versionBody } from "@/features/billing/lib/plans";
import type {
  BillingFeature,
  BillingPlan,
  BillingVersion,
  BillingVersionInput,
} from "@/features/billing/types";
import { cn } from "@/lib/utils";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

function VersionsWorkspace({
  plan,
  features,
  canWrite,
}: {
  plan: BillingPlan;
  features: BillingFeature[];
  canWrite: boolean;
}) {
  const { t, locale } = useLocale();
  const versions = useMemo(
    () => [...plan.versions].sort((a, b) => b.version - a.version),
    [plan.versions],
  );
  const [selected, setSelected] = useState<string | "new" | null>(
    () => versions[0]?.uuid ?? (canWrite ? "new" : null),
  );
  const current: BillingVersion | undefined = versions.find(
    (v) => v.uuid === selected,
  );
  // A draft based on `base` (published version or nothing) for "new".
  const [base, setBase] = useState<BillingVersion | undefined>(undefined);
  const [initial] = useState(() => draftFromVersion(features, versions[0]));
  const [draft, setDraft] = useState<BillingVersionInput>(initial.draft);
  const [filled, setFilled] = useState<string[]>(initial.filled);
  const [dirty, setDirty] = useState(false);
  const save = useSaveVersion(plan.uuid);
  const publish = usePublishVersion(plan.uuid);
  const [confirmPublish, setConfirmPublish] = useState(false);
  const [migrating, setMigrating] = useState(false);
  const readOnly =
    !canWrite || (selected !== "new" && current?.status !== "draft");
  const hasOpenDraft = versions.some((v) => v.status === "draft");
  const canMigrate =
    canWrite &&
    current?.status === "published" &&
    versions.some((v) => v.uuid !== current.uuid && v.status === "published");

  const select = (uuid: string | "new", from?: BillingVersion) => {
    setSelected(uuid);
    setBase(from);
    setDirty(false);
    const source =
      uuid === "new" ? from : versions.find((v) => v.uuid === uuid);
    const next = draftFromVersion(features, source);
    setDraft(next.draft);
    setFilled(next.filled);
  };

  const onChange = (next: BillingVersionInput) => {
    setDraft(next);
    setDirty(true);
  };

  const pricesValid =
    draft.prices.length > 0 &&
    new Set(draft.prices.map((p) => `${p.interval}:${p.currency}`)).size ===
      draft.prices.length;

  const saveDraft = async () => {
    const saved = await save.mutateAsync({
      version: selected === "new" ? null : (current?.uuid ?? null),
      body: versionBody(features, draft),
    });
    setSelected(saved.uuid);
    setBase(undefined);
    setDirty(false);
    return saved;
  };

  const heading =
    selected === "new"
      ? base
        ? t("billing.versions.new_from", { version: base.version })
        : t("billing.versions.new_draft")
      : t("billing.versions.version", { version: current?.version ?? 0 });

  return (
    <div className="space-y-5">
      <div className="space-y-2">
        <div className="-mx-1 overflow-x-auto px-1 pb-1">
          <ul
            className="flex w-max items-stretch gap-2"
            aria-label={t("billing.versions.title")}
          >
            {selected === "new" ? (
              <li>
                <span className="border-primary bg-primary/5 flex h-full items-center gap-2 rounded-lg border px-3 py-2 text-sm font-medium">
                  {t("billing.versions.new_draft")}
                  <Badge variant="warning">{t("billing.versions.draft")}</Badge>
                </span>
              </li>
            ) : null}
            {versions.map((v) => (
              <li key={v.uuid}>
                <button
                  type="button"
                  onClick={() => select(v.uuid)}
                  aria-current={selected === v.uuid ? "true" : undefined}
                  className={cn(
                    "hover:bg-muted/60 flex h-full flex-col items-start gap-0.5 rounded-lg border px-3 py-2 text-start text-sm transition-colors",
                    selected === v.uuid && "border-primary bg-primary/5",
                  )}
                >
                  <span className="flex items-center gap-2 font-medium">
                    {t("billing.versions.version", { version: v.version })}
                    <Badge
                      variant={v.status === "published" ? "success" : "warning"}
                    >
                      {t(`billing.versions.${v.status}`)}
                    </Badge>
                  </span>
                  <span className="text-muted-foreground text-xs">
                    {v.published_at
                      ? datetime(v.published_at, undefined, locale)
                      : t("credit_products.version.not_published")}
                  </span>
                </button>
              </li>
            ))}
            {canWrite && !hasOpenDraft && selected !== "new" ? (
              <li>
                <Button
                  type="button"
                  variant="outline"
                  className="h-full border-dashed"
                  onClick={() => select("new", current ?? versions[0])}
                >
                  <FilePlus2 className="size-4" />
                  {versions.length
                    ? t("billing.versions.new_from", {
                        version: (current ?? versions[0]).version,
                      })
                    : t("billing.versions.new_draft")}
                </Button>
              </li>
            ) : null}
          </ul>
        </div>
        <p className="text-muted-foreground text-xs">
          {t("credit_products.version.grandfathering")}
        </p>
      </div>

      {selected === null ? (
        <p className="text-muted-foreground text-sm">
          {t("billing.versions.none")}
        </p>
      ) : (
        <>
          <div className="flex flex-col gap-3 border-b pb-4 sm:flex-row sm:items-center sm:justify-between">
            <div>
              <h2 className="text-base font-semibold">{heading}</h2>
              <p className="text-muted-foreground text-xs">
                {readOnly
                  ? t("billing.versions.readonly_hint")
                  : t("billing.versions.draft_hint")}
              </p>
            </div>
            <div className="flex flex-wrap gap-2">
              {canMigrate ? (
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={() => setMigrating(true)}
                >
                  <Users className="size-4" />
                  {t("credit_products.migrate.open")}
                </Button>
              ) : null}
              {!readOnly ? (
                <>
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    disabled={
                      !pricesValid ||
                      save.isPending ||
                      (!dirty && selected !== "new")
                    }
                    onClick={() => void saveDraft()}
                  >
                    <Save className="size-4" />
                    {t("billing.versions.save")}
                  </Button>
                  <Button
                    type="button"
                    size="sm"
                    disabled={
                      !pricesValid || save.isPending || publish.isPending
                    }
                    onClick={() => setConfirmPublish(true)}
                  >
                    <Rocket className="size-4" />
                    {t("billing.versions.publish")}
                  </Button>
                </>
              ) : null}
            </div>
          </div>
          <VersionEditor
            key={`${selected}-${base?.uuid ?? ""}`}
            features={features}
            value={draft}
            onChange={onChange}
            readOnly={readOnly}
            planName={plan.name}
            filled={readOnly ? [] : filled}
          />
        </>
      )}

      <ConfirmDialog
        open={confirmPublish}
        title={t("billing.versions.publish_title")}
        description={t("credit_products.version.publish_description")}
        confirmLabel={t("billing.versions.publish")}
        isPending={save.isPending || publish.isPending}
        onCancel={() => setConfirmPublish(false)}
        onConfirm={async () => {
          try {
            const target =
              selected === "new" || dirty
                ? await saveDraft()
                : (current as BillingVersion);
            const published = await publish.mutateAsync(target.uuid);
            setSelected(published.uuid);
            setDirty(false);
          } finally {
            setConfirmPublish(false);
          }
        }}
      />
      {canMigrate && current ? (
        <MigrateSubscribersDialog
          open={migrating}
          onClose={() => setMigrating(false)}
          plan={plan}
          target={current}
        />
      ) : null}
    </div>
  );
}

/**
 * Plan detail (CF-39): identity (name, code, visibility, order), archive,
 * and the version workspace (draft → publish). Published versions are
 * immutable; existing subscriptions keep their pinned version.
 */
export function PlanEditorPage({ uuid }: { uuid: string }) {
  const { t } = useLocale();
  const { can } = usePermission();
  const router = useRouter();
  const plan = usePlan(uuid);
  const features = useBillingFeatures();
  const update = useUpdatePlan(uuid);
  const archive = useArchivePlan(uuid);
  const [confirmArchive, setConfirmArchive] = useState(false);
  const canWrite = can(permissions.billing.plansWrite);
  const schema = useMemo(() => createPlanSchema(t, false), [t]);
  const data = plan.data;
  const title = data?.name ?? t("billing.plans.detail_title");

  return (
    <EntityPage
      title={title}
      description={t("billing.plans.detail_description")}
      permission={permissions.billing.plansRead}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("billing.admin.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.platform.home },
        {
          label: t("billing.admin.title"),
          href: routes.platform.billing.tab("plans"),
        },
        { label: title },
      ]}
      actions={
        data && canWrite && !data.archived_at ? (
          <Button
            type="button"
            size="sm"
            variant="outline"
            className="text-destructive"
            onClick={() => setConfirmArchive(true)}
          >
            <Archive className="size-4" />
            {t("billing.plans.archive")}
          </Button>
        ) : null
      }
    >
      {plan.isLoading || features.isLoading ? (
        <Loading label={t("common.loading")} />
      ) : null}
      {plan.isError || features.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          onRetry={() => {
            void plan.refetch();
            void features.refetch();
          }}
          retryLabel={t("common.retry")}
        />
      ) : null}
      {data && features.data ? (
        <div className="space-y-6">
          {data.archived_at ? (
            <p className="bg-muted rounded-md px-3 py-2 text-sm">
              {t("billing.plans.archived_hint")}
            </p>
          ) : null}
          <EntitySectionCard title={t("billing.plans.identity")}>
            <AppForm<PlanFormValues>
              key={`${data.name}|${data.visibility}|${data.sort_order}`}
              className="space-y-4"
              schema={schema}
              defaultValues={{
                code: data.code ?? "",
                name: data.name,
                description: data.description ?? "",
                visibility: data.visibility ?? "public",
                sort_order: data.sort_order ?? 0,
              }}
              onSubmit={async (values) => {
                await update.mutateAsync({
                  code: data.code,
                  name: values.name.trim(),
                  description: values.description,
                  visibility: values.visibility,
                  sort_order: values.sort_order,
                });
              }}
            >
              {(form) => (
                <PlanFields
                  create={false}
                  submitting={form.formState.isSubmitting}
                  disabled={!canWrite}
                />
              )}
            </AppForm>
          </EntitySectionCard>
          <EntitySectionCard title={t("billing.versions.title")}>
            <VersionsWorkspace
              key={data.versions.map((v) => `${v.uuid}:${v.status}`).join(",")}
              plan={data}
              features={features.data}
              canWrite={canWrite && !data.archived_at}
            />
          </EntitySectionCard>
        </div>
      ) : null}
      <ConfirmDialog
        open={confirmArchive}
        title={t("billing.plans.archive_title", { name: data?.name ?? "" })}
        description={t("billing.plans.archive_description")}
        confirmLabel={t("billing.plans.archive")}
        variant="destructive"
        isPending={archive.isPending}
        onCancel={() => setConfirmArchive(false)}
        onConfirm={() =>
          archive.mutate(undefined, {
            onSuccess: () => router.push(routes.platform.billing.tab("plans")),
            onSettled: () => setConfirmArchive(false),
          })
        }
      />
    </EntityPage>
  );
}
