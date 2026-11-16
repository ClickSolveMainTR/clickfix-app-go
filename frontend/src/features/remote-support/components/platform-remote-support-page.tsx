"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { MonitorUp, Power, PowerOff } from "lucide-react";
import { useState } from "react";

import { AppDialog } from "@/components/dialogs/app-dialog";
import { PageHeader } from "@/components/layout/page-header";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Textarea } from "@/components/ui/textarea";
import { cn, datetime } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

import { errorKey } from "../lib/access";
import {
  platformRemoteSupportService,
  type RemoteSupportPlatformOrganization,
} from "../services/remote-support.service";

const KEY = ["platform-remote-support"] as const;
const LIMIT = 25;

/**
 * Platform abuse controls (platform.remote_support.manage): per
 * organization kill switch and the reported sessions.
 */
export function PlatformRemoteSupportPage() {
  const { t } = useLocale();
  return (
    <div className="space-y-6">
      <PageHeader
        icon={<MonitorUp className="size-7" />}
        title={t("remote_support.platform.title")}
        description={t("remote_support.platform.description")}
      />
      <Tabs defaultValue="organizations">
        <TabsList>
          <TabsTrigger value="organizations">
            {t("remote_support.platform.tabs.organizations")}
          </TabsTrigger>
          <TabsTrigger value="reports">
            {t("remote_support.platform.tabs.reports")}
          </TabsTrigger>
        </TabsList>
        <TabsContent value="organizations" className="mt-4">
          <Organizations />
        </TabsContent>
        <TabsContent value="reports" className="mt-4">
          <Reports />
        </TabsContent>
      </Tabs>
    </div>
  );
}

function Pager({
  offset,
  count,
  onOffset,
}: {
  offset: number;
  count: number;
  onOffset: (n: number) => void;
}) {
  const { t } = useLocale();
  return (
    <div className="flex items-center justify-end gap-2">
      <Button
        size="sm"
        variant="outline"
        disabled={offset === 0}
        onClick={() => onOffset(Math.max(0, offset - LIMIT))}
      >
        {t("remote_support.platform.previous")}
      </Button>
      <Button
        size="sm"
        variant="outline"
        disabled={count < LIMIT}
        onClick={() => onOffset(offset + LIMIT)}
      >
        {t("remote_support.platform.next")}
      </Button>
    </div>
  );
}

const th = "text-muted-foreground px-3 py-2 text-left text-xs font-medium";
const td = "px-3 py-2 align-top";

function Organizations() {
  const { t, locale } = useLocale();
  const [state, setState] = useState("all");
  const [offset, setOffset] = useState(0);
  const [target, setTarget] =
    useState<RemoteSupportPlatformOrganization | null>(null);
  const query = useQuery({
    queryKey: [...KEY, "organizations", state, offset],
    queryFn: () =>
      platformRemoteSupportService.organizations({
        state: state === "all" ? undefined : state,
        limit: LIMIT,
        offset,
      }),
  });
  const items = query.data?.items ?? [];
  return (
    <div className="space-y-3">
      <div className="flex items-center gap-2">
        <Label htmlFor="rs-state-filter" className="text-sm">
          {t("remote_support.platform.state_filter")}
        </Label>
        <Select
          value={state}
          onValueChange={(v) => {
            setState(v);
            setOffset(0);
          }}
        >
          <SelectTrigger id="rs-state-filter" className="w-48">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {["all", "active", "disabled", "suspended"].map((s) => (
              <SelectItem key={s} value={s}>
                {t(`remote_support.platform.state.${s}`)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <div className="overflow-x-auto rounded-lg border">
        <table className="w-full text-sm">
          <caption className="sr-only">
            {t("remote_support.platform.tabs.organizations")}
          </caption>
          <thead className="bg-muted/40">
            <tr>
              <th className={th}>
                {t("remote_support.platform.col.organization")}
              </th>
              <th className={th}>{t("remote_support.platform.col.enabled")}</th>
              <th className={th}>{t("remote_support.platform.col.state")}</th>
              <th className={th}>
                {t("remote_support.platform.col.sessions")}
              </th>
              <th className={th}>{t("remote_support.platform.col.reports")}</th>
              <th className={th}>
                <span className="sr-only">
                  {t("remote_support.platform.col.actions")}
                </span>
              </th>
            </tr>
          </thead>
          <tbody>
            {items.map((o) => (
              <tr key={o.uuid} className="border-t">
                <td className={td}>
                  <p className="font-medium">{o.name}</p>
                  <p className="text-muted-foreground text-xs">{o.slug}</p>
                </td>
                <td className={td}>
                  {o.enabled
                    ? t("remote_support.platform.yes")
                    : t("remote_support.platform.no")}
                </td>
                <td className={td}>
                  <span
                    className={cn(
                      "rounded px-1.5 py-0.5 text-xs",
                      o.platform_state === "active"
                        ? "bg-emerald-500/10 text-emerald-800 dark:text-emerald-200"
                        : "bg-red-500/10 text-red-800 dark:text-red-200",
                    )}
                  >
                    {t(`remote_support.platform.state.${o.platform_state}`)}
                  </span>
                  {o.platform_reason ? (
                    <p className="text-muted-foreground mt-1 text-xs">
                      {o.platform_reason}
                    </p>
                  ) : null}
                  {o.platform_changed_at ? (
                    <p className="text-muted-foreground text-xs">
                      {datetime(
                        o.platform_changed_at,
                        "dd.MM.yyyy HH:mm",
                        locale,
                      )}
                    </p>
                  ) : null}
                </td>
                <td className={td}>{o.sessions_30d}</td>
                <td
                  className={cn(
                    td,
                    o.reports_30d > 0 && "font-semibold text-red-600",
                  )}
                >
                  {o.reports_30d}
                </td>
                <td className={cn(td, "text-right")}>
                  <Button
                    size="sm"
                    variant={
                      o.platform_state === "active" ? "destructive" : "outline"
                    }
                    onClick={() => setTarget(o)}
                  >
                    {o.platform_state === "active" ? (
                      <PowerOff className="size-4" />
                    ) : (
                      <Power className="size-4" />
                    )}
                    {o.platform_state === "active"
                      ? t("remote_support.platform.disable")
                      : t("remote_support.platform.enable")}
                  </Button>
                </td>
              </tr>
            ))}
            {!query.isPending && items.length === 0 ? (
              <tr>
                <td
                  colSpan={6}
                  className="text-muted-foreground px-3 py-6 text-center"
                >
                  {t("remote_support.platform.empty")}
                </td>
              </tr>
            ) : null}
          </tbody>
        </table>
      </div>
      <Pager offset={offset} count={items.length} onOffset={setOffset} />
      {target ? (
        <StateDialog org={target} onClose={() => setTarget(null)} />
      ) : null}
    </div>
  );
}

function StateDialog({
  org,
  onClose,
}: {
  org: RemoteSupportPlatformOrganization;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const disabling = org.platform_state === "active";
  const [reason, setReason] = useState("");
  const save = useMutation({
    mutationFn: () =>
      platformRemoteSupportService.setState(org.uuid, {
        state: disabling ? "disabled" : "active",
        reason: reason.trim() || undefined,
      }),
    onSuccess: () => {
      appToast.success(t("remote_support.platform.saved"));
      void qc.invalidateQueries({ queryKey: KEY });
      onClose();
    },
    onError: (err) =>
      appToast.error(t(`remote_support.errors.${errorKey(err)}`)),
  });
  return (
    <AppDialog
      open
      onOpenChange={(o) => !o && onClose()}
      title={
        disabling
          ? t("remote_support.platform.disable_title", { name: org.name })
          : t("remote_support.platform.enable_title", { name: org.name })
      }
      description={
        disabling
          ? t("remote_support.platform.disable_hint")
          : t("remote_support.platform.enable_hint")
      }
      footer={
        <>
          <Button variant="outline" onClick={onClose}>
            {t("common.cancel")}
          </Button>
          <Button
            variant={disabling ? "destructive" : "default"}
            disabled={save.isPending || (disabling && reason.trim().length < 3)}
            onClick={() => save.mutate()}
          >
            {disabling
              ? t("remote_support.platform.disable")
              : t("remote_support.platform.enable")}
          </Button>
        </>
      }
    >
      <div className="space-y-1.5">
        <Label htmlFor="rs-state-reason">
          {t("remote_support.platform.reason")}
        </Label>
        <Textarea
          id="rs-state-reason"
          value={reason}
          maxLength={500}
          required={disabling}
          onChange={(e) => setReason(e.target.value)}
        />
      </div>
    </AppDialog>
  );
}

function Reports() {
  const { t, locale } = useLocale();
  const [offset, setOffset] = useState(0);
  const query = useQuery({
    queryKey: [...KEY, "reports", offset],
    queryFn: () =>
      platformRemoteSupportService.reports({ limit: LIMIT, offset }),
  });
  const items = query.data?.items ?? [];
  return (
    <div className="space-y-3">
      <div className="overflow-x-auto rounded-lg border">
        <table className="w-full text-sm">
          <caption className="sr-only">
            {t("remote_support.platform.tabs.reports")}
          </caption>
          <thead className="bg-muted/40">
            <tr>
              <th className={th}>
                {t("remote_support.platform.col.reported_at")}
              </th>
              <th className={th}>
                {t("remote_support.platform.col.organization")}
              </th>
              <th className={th}>{t("remote_support.platform.col.agent")}</th>
              <th className={th}>{t("remote_support.platform.col.mode")}</th>
              <th className={th}>{t("remote_support.platform.col.reason")}</th>
              <th className={th}>{t("remote_support.platform.col.session")}</th>
            </tr>
          </thead>
          <tbody>
            {items.map((r) => (
              <tr key={r.session_uuid} className="border-t">
                <td className={td}>
                  {datetime(r.reported_at, "dd.MM.yyyy HH:mm", locale)}
                </td>
                <td className={td}>{r.organization_name}</td>
                <td className={td}>{r.agent_name || "—"}</td>
                <td className={td}>
                  {t(`remote_support.mode.${r.mode_requested}`)}
                </td>
                <td className={cn(td, "max-w-xs whitespace-pre-wrap")}>
                  {r.report_reason || "—"}
                </td>
                <td className={td}>
                  <code className="text-xs">{r.session_uuid.slice(0, 8)}</code>
                  <p className="text-muted-foreground text-xs">
                    {t(`remote_support.status.${r.status}`)}
                  </p>
                </td>
              </tr>
            ))}
            {!query.isPending && items.length === 0 ? (
              <tr>
                <td
                  colSpan={6}
                  className="text-muted-foreground px-3 py-6 text-center"
                >
                  {t("remote_support.platform.no_reports")}
                </td>
              </tr>
            ) : null}
          </tbody>
        </table>
      </div>
      <Pager offset={offset} count={items.length} onOffset={setOffset} />
    </div>
  );
}
