"use client";

import {
  Check,
  CircleCheck,
  CircleX,
  Copy,
  ExternalLink,
  FileUp,
  KeyRound,
  Lock,
  Network,
  Plus,
  RefreshCw,
  ShieldCheck,
  TriangleAlert,
  Users,
  X,
} from "lucide-react";
import { useSearchParams } from "next/navigation";
import { useMemo, useRef, useState } from "react";
import { z } from "zod";

import { ErrorState } from "@/components/common/error-state";
import { AppDialog } from "@/components/dialogs/app-dialog";
import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import { EntityPage, EntitySectionCard } from "@/components/entity";
import {
  AppForm,
  AppInput,
  AppPassword,
  AppRadioGroup,
  AppSwitch,
  AppTextarea,
  FormActions,
} from "@/components/forms";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { allowedBy, normalizeCIDR } from "@/features/security-settings/cidr";
import {
  spUrls,
  useClientIP,
  useRotateSCIM,
  useSSOConfig,
  useSaveSSOConfig,
  type SSOConfig,
} from "@/features/security-settings/security";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { startOrganizationSSO } from "@/lib/auth/organization-sso";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

type Translate = (
  key: string,
  vars?: Record<string, string | number>,
) => string;

function CopyField({ label, value }: { label: string; value: string }) {
  const { t } = useLocale();
  const [copied, setCopied] = useState(false);
  return (
    <div className="space-y-1.5">
      <Label className="text-xs">{label}</Label>
      <div className="flex gap-2">
        <Input readOnly value={value} className="font-mono text-xs" />
        <Button
          type="button"
          variant="outline"
          size="icon"
          aria-label={t("security.copy")}
          onClick={async () => {
            try {
              await navigator.clipboard.writeText(value);
              setCopied(true);
              setTimeout(() => setCopied(false), 1500);
            } catch {
              appToast.error(t("security.copy_failed"));
            }
          }}
        >
          {copied ? <Check className="size-4" /> : <Copy className="size-4" />}
        </Button>
      </div>
    </div>
  );
}

const httpsURL = (t: Translate) =>
  z
    .string()
    .trim()
    .refine((v) => {
      try {
        const u = new URL(v);
        return u.protocol === "https:" && !u.hash && !u.username;
      } catch {
        return false;
      }
    }, t("security.validation.https"));

function createSSOSchema(t: Translate) {
  return z
    .object({
      enabled: z.boolean(),
      protocol: z.enum(["oidc", "saml"]),
      issuer: z.string(),
      client_id: z.string(),
      client_secret: z.string(),
      authorization_url: z.string(),
      token_url: z.string(),
      jwks_url: z.string(),
      saml_metadata: z.string().max(1_000_000, t("security.validation.xml")),
    })
    .superRefine((v, ctx) => {
      if (!v.enabled) return;
      if (v.protocol === "saml") {
        if (!v.saml_metadata.trim().startsWith("<")) {
          ctx.addIssue({
            code: "custom",
            path: ["saml_metadata"],
            message: t("security.validation.xml"),
          });
        }
        return;
      }
      const url = httpsURL(t);
      for (const key of [
        "issuer",
        "authorization_url",
        "token_url",
        "jwks_url",
      ] as const) {
        const r = url.safeParse(v[key]);
        if (!r.success) {
          ctx.addIssue({
            code: "custom",
            path: [key],
            message: r.error.issues[0]?.message ?? "",
          });
        }
      }
      if (!v.client_id.trim()) {
        ctx.addIssue({
          code: "custom",
          path: ["client_id"],
          message: t("security.validation.required"),
        });
      }
    });
}
type SSOValues = z.infer<ReturnType<typeof createSSOSchema>>;

function SAMLMetadataUpload({ onLoad }: { onLoad: (xml: string) => void }) {
  const { t } = useLocale();
  const input = useRef<HTMLInputElement>(null);
  return (
    <>
      <input
        ref={input}
        type="file"
        accept=".xml,application/xml,text/xml,application/samlmetadata+xml"
        className="hidden"
        onChange={async (e) => {
          const file = e.target.files?.[0];
          e.target.value = "";
          if (!file) return;
          if (file.size > 1_000_000) {
            appToast.error(t("security.validation.xml"));
            return;
          }
          onLoad(await file.text());
        }}
      />
      <Button
        type="button"
        variant="outline"
        size="sm"
        onClick={() => input.current?.click()}
      >
        <FileUp className="size-4" />
        {t("security.sso.upload_metadata")}
      </Button>
    </>
  );
}

function SSOCard({ config, org }: { config: SSOConfig; org: string }) {
  const { t } = useLocale();
  const save = useSaveSSOConfig("security.toast.sso_saved");
  const schema = useMemo(() => createSSOSchema(t), [t]);
  const urls = spUrls(org);
  const [testing, setTesting] = useState(false);
  return (
    <EntitySectionCard
      title={t("security.sso.title")}
      description={t("security.sso.description")}
    >
      <AppForm<SSOValues>
        key={JSON.stringify(config)}
        className="space-y-5"
        schema={schema}
        defaultValues={{
          enabled: config.enabled,
          protocol: config.protocol,
          issuer: config.issuer ?? "",
          client_id: config.client_id ?? "",
          client_secret: "",
          authorization_url: config.authorization_url ?? "",
          token_url: config.token_url ?? "",
          jwks_url: config.jwks_url ?? "",
          saml_metadata: config.saml_metadata ?? "",
        }}
        onSubmit={async (v) => {
          await save.mutateAsync({
            ...config,
            ...v,
            // Enforcement needs SSO on; turning SSO off lifts it.
            enforced: v.enabled && config.enforced,
            ip_allowlist: config.ip_allowlist ?? [],
          });
        }}
      >
        {(form) => {
          const protocol = form.watch("protocol");
          const dirty = form.formState.isDirty;
          return (
            <>
              <AppSwitch
                name="enabled"
                label={t("security.sso.enabled")}
                description={t("security.sso.enabled_hint")}
              />
              <AppRadioGroup
                name="protocol"
                label={t("security.sso.protocol")}
                options={[
                  { value: "oidc", label: "OpenID Connect (OIDC)" },
                  { value: "saml", label: "SAML 2.0" },
                ]}
              />
              {protocol === "oidc" ? (
                <div className="grid gap-4 md:grid-cols-2">
                  <AppInput
                    name="issuer"
                    label={t("security.sso.issuer")}
                    placeholder="https://login.example.com"
                    className="md:col-span-2"
                  />
                  <AppInput
                    name="client_id"
                    label={t("security.sso.client_id")}
                    autoComplete="off"
                  />
                  <AppPassword
                    name="client_secret"
                    label={t("security.sso.client_secret")}
                    description={t("security.sso.client_secret_hint")}
                    autoComplete="new-password"
                  />
                  <AppInput
                    name="authorization_url"
                    label={t("security.sso.authorization_url")}
                  />
                  <AppInput
                    name="token_url"
                    label={t("security.sso.token_url")}
                  />
                  <AppInput
                    name="jwks_url"
                    label={t("security.sso.jwks_url")}
                    className="md:col-span-2"
                  />
                  <div className="md:col-span-2">
                    <CopyField
                      label={t("security.sso.redirect_uri")}
                      value={urls.oidcRedirect}
                    />
                  </div>
                </div>
              ) : (
                <div className="space-y-4">
                  <div className="flex flex-wrap items-center justify-between gap-2">
                    <p className="text-muted-foreground text-sm">
                      {t("security.sso.metadata_hint")}
                    </p>
                    <SAMLMetadataUpload
                      onLoad={(xml) =>
                        form.setValue("saml_metadata", xml, {
                          shouldDirty: true,
                          shouldValidate: true,
                        })
                      }
                    />
                  </div>
                  <AppTextarea
                    name="saml_metadata"
                    label={t("security.sso.metadata")}
                    rows={6}
                    className="font-mono text-xs"
                    spellCheck={false}
                  />
                  <div className="grid gap-4 md:grid-cols-2">
                    <CopyField
                      label={t("security.sso.acs_url")}
                      value={urls.samlAcs}
                    />
                    <CopyField
                      label={t("security.sso.entity_id")}
                      value={urls.samlMetadata}
                    />
                  </div>
                </div>
              )}
              {config.enabled ? (
                <CopyField
                  label={t("security.sso.login_link")}
                  value={urls.signIn}
                />
              ) : null}
              <FormActions
                sticky={false}
                className="border-0 bg-transparent p-0"
              >
                <Button
                  type="button"
                  variant="outline"
                  disabled={!config.enabled || dirty || testing}
                  title={dirty ? t("security.sso.test_save_first") : undefined}
                  onClick={async () => {
                    setTesting(true);
                    try {
                      window.location.assign(
                        await startOrganizationSSO(
                          org,
                          routes.cms.settings.security,
                        ),
                      );
                    } catch {
                      appToast.error(t("security.sso.test_start_failed"));
                      setTesting(false);
                    }
                  }}
                >
                  <ExternalLink className="size-4" />
                  {t("security.sso.test")}
                </Button>
                <Button type="submit" disabled={save.isPending}>
                  {t("common.save")}
                </Button>
              </FormActions>
            </>
          );
        }}
      </AppForm>
    </EntitySectionCard>
  );
}

function EnforceCard({ config }: { config: SSOConfig }) {
  const { t } = useLocale();
  const save = useSaveSSOConfig(
    "security.toast.enforce_saved",
    "security.errors.enforce_needs_sso",
  );
  const [pending, setPending] = useState<boolean | null>(null);
  const apply = (enforced: boolean) =>
    save.mutate(
      { ...config, enforced, ip_allowlist: config.ip_allowlist ?? [] },
      { onSettled: () => setPending(null) },
    );
  return (
    <EntitySectionCard
      title={t("security.enforce.title")}
      description={t("security.enforce.description")}
    >
      <div className="space-y-4">
        <div className="flex items-start justify-between gap-4">
          <div className="space-y-0.5">
            <Label htmlFor="sso-enforced">{t("security.enforce.label")}</Label>
            <p className="text-muted-foreground text-xs">
              {config.enabled
                ? t("security.enforce.hint")
                : t("security.enforce.needs_sso")}
            </p>
          </div>
          <Switch
            id="sso-enforced"
            checked={config.enforced}
            disabled={(!config.enabled && !config.enforced) || save.isPending}
            onCheckedChange={(next) => setPending(next)}
          />
        </div>
        <Alert>
          <Lock className="size-4" />
          <AlertDescription>
            {t("security.enforce.session_note")}
          </AlertDescription>
        </Alert>
      </div>
      <ConfirmDialog
        open={pending !== null}
        title={t(
          pending ? "security.enforce.on_title" : "security.enforce.off_title",
        )}
        description={t(
          pending
            ? "security.enforce.on_description"
            : "security.enforce.off_description",
        )}
        confirmLabel={t(
          pending
            ? "security.enforce.on_confirm"
            : "security.enforce.off_confirm",
        )}
        variant={pending ? "destructive" : "default"}
        isPending={save.isPending}
        onCancel={() => setPending(null)}
        onConfirm={() => pending !== null && apply(pending)}
      />
    </EntitySectionCard>
  );
}

function SCIMCard({ org }: { org: string }) {
  const { t } = useLocale();
  const rotate = useRotateSCIM();
  const [confirm, setConfirm] = useState(false);
  const [issued, setIssued] = useState<{
    token: string;
    base_url: string;
  } | null>(null);
  return (
    <EntitySectionCard
      title={t("security.scim.title")}
      description={t("security.scim.description")}
    >
      <div className="space-y-4">
        <CopyField
          label={t("security.scim.base_url")}
          value={spUrls(org).scimBase}
        />
        <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <p className="text-muted-foreground text-sm">
            {t("security.scim.token_hint")}
          </p>
          <Button
            type="button"
            variant="outline"
            onClick={() => setConfirm(true)}
            disabled={rotate.isPending}
          >
            <RefreshCw className="size-4" />
            {t("security.scim.rotate")}
          </Button>
        </div>
      </div>
      <ConfirmDialog
        open={confirm}
        title={t("security.scim.rotate_title")}
        description={t("security.scim.rotate_description")}
        confirmLabel={t("security.scim.rotate_confirm")}
        variant="destructive"
        isPending={rotate.isPending}
        onCancel={() => setConfirm(false)}
        onConfirm={() =>
          rotate.mutate(undefined, {
            onSuccess: (data) => setIssued(data),
            onSettled: () => setConfirm(false),
          })
        }
      />
      <AppDialog
        open={issued !== null}
        onOpenChange={(open) => !open && setIssued(null)}
        title={t("security.scim.issued_title")}
        description={t("security.scim.issued_description")}
        footer={
          <Button type="button" onClick={() => setIssued(null)}>
            {t("security.scim.issued_done")}
          </Button>
        }
      >
        {issued ? (
          <div className="space-y-4">
            <Alert>
              <KeyRound className="size-4" />
              <AlertDescription>
                {t("security.scim.shown_once")}
              </AlertDescription>
            </Alert>
            <CopyField label={t("security.scim.token")} value={issued.token} />
            <CopyField
              label={t("security.scim.base_url")}
              value={issued.base_url}
            />
          </div>
        ) : null}
      </AppDialog>
    </EntitySectionCard>
  );
}

function AllowlistCard({ config }: { config: SSOConfig }) {
  const { t } = useLocale();
  const save = useSaveSSOConfig("security.toast.allowlist_saved");
  const ip = useClientIP();
  const saved = useMemo(() => config.ip_allowlist ?? [], [config.ip_allowlist]);
  const [list, setList] = useState<string[]>(saved);
  const [draft, setDraft] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [confirm, setConfirm] = useState(false);
  const current = ip.data?.ip ?? "";
  const dirty = list.join("\n") !== saved.join("\n");
  const excludesMe = Boolean(current) && !allowedBy(list, current);

  const add = (raw: string) => {
    const cidr = normalizeCIDR(raw);
    if (!cidr) {
      setError(t("security.allowlist.invalid"));
      return;
    }
    if (list.length >= 100) {
      setError(t("security.allowlist.too_many"));
      return;
    }
    setError(null);
    setDraft("");
    if (!list.includes(cidr)) setList([...list, cidr]);
  };

  return (
    <EntitySectionCard
      title={t("security.allowlist.title")}
      description={t("security.allowlist.description")}
    >
      <div className="space-y-4">
        <div className="flex flex-wrap items-center gap-2 text-sm">
          <span className="text-muted-foreground">
            {t("security.allowlist.current_ip")}
          </span>
          {current ? (
            <>
              <code className="bg-muted rounded px-1.5 py-0.5 text-xs">
                {current}
              </code>
              <Badge variant={allowedBy(saved, current) ? "success" : "danger"}>
                {allowedBy(saved, current)
                  ? t("security.allowlist.current_allowed")
                  : t("security.allowlist.current_blocked")}
              </Badge>
              <Button
                type="button"
                size="sm"
                variant="ghost"
                onClick={() => add(current)}
                disabled={!allowedBy(list, current) ? false : list.length > 0}
              >
                <Plus className="size-4" />
                {t("security.allowlist.add_current")}
              </Button>
            </>
          ) : (
            <Skeleton className="h-5 w-28" />
          )}
        </div>
        <form
          className="flex flex-col gap-2 sm:flex-row"
          onSubmit={(e) => {
            e.preventDefault();
            add(draft);
          }}
        >
          <div className="flex-1 space-y-1">
            <Input
              value={draft}
              onChange={(e) => {
                setDraft(e.target.value);
                setError(null);
              }}
              placeholder="203.0.113.0/24"
              aria-label={t("security.allowlist.input")}
              aria-invalid={Boolean(error)}
              className="font-mono"
            />
            {error ? <p className="text-destructive text-xs">{error}</p> : null}
          </div>
          <Button type="submit" variant="outline" disabled={!draft.trim()}>
            <Plus className="size-4" />
            {t("security.allowlist.add")}
          </Button>
        </form>
        {list.length ? (
          <ul className="divide-y rounded-lg border">
            {list.map((cidr) => (
              <li
                key={cidr}
                className="flex items-center justify-between gap-2 px-3 py-2"
              >
                <code className="truncate text-sm">{cidr}</code>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  aria-label={t("security.allowlist.remove", { cidr })}
                  onClick={() => setList(list.filter((c) => c !== cidr))}
                >
                  <X className="size-4" />
                </Button>
              </li>
            ))}
          </ul>
        ) : (
          <p className="text-muted-foreground rounded-lg border border-dashed p-4 text-center text-sm">
            {t("security.allowlist.empty")}
          </p>
        )}
        {list.length && excludesMe ? (
          <Alert variant="destructive">
            <TriangleAlert className="size-4" />
            <AlertTitle>{t("security.allowlist.lockout_title")}</AlertTitle>
            <AlertDescription>
              {t("security.allowlist.lockout_body", { ip: current })}
            </AlertDescription>
          </Alert>
        ) : null}
        <FormActions sticky={false} className="border-0 bg-transparent p-0">
          {dirty ? (
            <Button
              type="button"
              variant="outline"
              onClick={() => {
                setList(saved);
                setError(null);
              }}
            >
              {t("common.cancel")}
            </Button>
          ) : null}
          <Button
            type="button"
            disabled={
              !dirty || save.isPending || (list.length > 0 && excludesMe)
            }
            onClick={() => setConfirm(true)}
          >
            {t("common.save")}
          </Button>
        </FormActions>
      </div>
      <ConfirmDialog
        open={confirm}
        title={t("security.allowlist.confirm_title")}
        description={t(
          list.length
            ? "security.allowlist.confirm_description"
            : "security.allowlist.confirm_clear",
          { count: list.length },
        )}
        confirmLabel={t("common.save")}
        isPending={save.isPending}
        onCancel={() => setConfirm(false)}
        onConfirm={() =>
          save.mutate(
            { ...config, ip_allowlist: list },
            { onSettled: () => setConfirm(false) },
          )
        }
      />
    </EntitySectionCard>
  );
}

function TestResult() {
  const { t } = useLocale();
  // Set by the SSO callback (BFF) when the test started here.
  const result = useSearchParams().get("sso");
  if (result !== "ok" && result !== "failed") return null;
  const ok = result === "ok";
  return (
    <Alert variant={ok ? "default" : "destructive"}>
      {ok ? (
        <CircleCheck className="size-4 text-emerald-600 dark:text-emerald-400" />
      ) : (
        <CircleX className="size-4" />
      )}
      <AlertTitle>
        {t(
          ok ? "security.sso.test_ok_title" : "security.sso.test_failed_title",
        )}
      </AlertTitle>
      <AlertDescription>
        {t(ok ? "security.sso.test_ok_body" : "security.sso.test_failed_body")}
      </AlertDescription>
    </Alert>
  );
}

/**
 * Organization security (CF-128): SSO (OIDC / SAML), enforced SSO, SCIM
 * provisioning and the IP allow-list. All endpoints need
 * `tenant.settings.write` and the `security.sso` plan feature.
 */
export function SecurityPage() {
  const { t } = useLocale();
  const { organization } = useTenant();
  const query = useSSOConfig();
  const org = organization?.uuid ?? "";
  return (
    <EntityPage
      title={t("security.title")}
      description={t("security.description")}
      permission={permissions.tenant.settings.write}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("security.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.cms.home },
        {
          label: t("layout.section_tenant_settings"),
          href: routes.cms.settings.organization,
        },
        { label: t("security.title") },
      ]}
    >
      <div className="space-y-6">
        <TestResult />
        {query.isLoading || !org ? (
          <Skeleton className="h-96" />
        ) : query.isError || !query.data ? (
          <ErrorState
            title={t("security.load_failed")}
            onRetry={() => void query.refetch()}
          />
        ) : (
          <>
            <div className="flex flex-wrap gap-2">
              <Badge variant={query.data.enabled ? "success" : "secondary"}>
                <ShieldCheck className="size-3" />
                {t(
                  query.data.enabled
                    ? "security.badges.sso_on"
                    : "security.badges.sso_off",
                )}
              </Badge>
              {query.data.enforced ? (
                <Badge variant="warning">
                  <Lock className="size-3" />
                  {t("security.badges.enforced")}
                </Badge>
              ) : null}
              {query.data.ip_allowlist?.length ? (
                <Badge variant="secondary">
                  <Network className="size-3" />
                  {t("security.badges.allowlist", {
                    count: query.data.ip_allowlist.length,
                  })}
                </Badge>
              ) : null}
            </div>
            <SSOCard config={query.data} org={org} />
            <EnforceCard config={query.data} />
            <AllowlistCard
              key={(query.data.ip_allowlist ?? []).join(",")}
              config={query.data}
            />
            <SCIMCard org={org} />
            <p className="text-muted-foreground flex items-center gap-2 text-xs">
              <Users className="size-3.5" />
              {t("security.plan_note")}
            </p>
          </>
        )}
      </div>
    </EntityPage>
  );
}
