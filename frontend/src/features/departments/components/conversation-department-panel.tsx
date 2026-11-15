"use client";

import { ArrowRightLeft, Sparkles } from "lucide-react";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { permissions } from "@/config/permissions";
import {
  useConversationDepartment,
  useDepartmentOptions,
  useTransferConversation,
} from "@/features/departments/hooks/use-departments";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

const ORG_WIDE = "none";

function TransferDialog({
  conversation,
  current,
  open,
  onClose,
}: {
  conversation: string;
  current?: string;
  open: boolean;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const { options } = useDepartmentOptions();
  const transfer = useTransferConversation(conversation);
  const [target, setTarget] = useState("");
  const [reason, setReason] = useState("");
  const [summary, setSummary] = useState("");
  const [release, setRelease] = useState(false);
  const targets = options.filter((o) => o.value !== current);
  return (
    <Dialog open={open} onOpenChange={(next) => !next && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("departments.transfer.title")}</DialogTitle>
          <DialogDescription>
            {t("departments.transfer.description")}
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="transfer-target">
              {t("departments.transfer.target")}
            </Label>
            <Select value={target} onValueChange={setTarget}>
              <SelectTrigger id="transfer-target" className="w-full">
                <SelectValue
                  placeholder={t("departments.transfer.target_placeholder")}
                />
              </SelectTrigger>
              <SelectContent>
                {current ? (
                  <SelectItem value={ORG_WIDE}>
                    {t("departments.transfer.organization_wide")}
                  </SelectItem>
                ) : null}
                {targets.map((o) => (
                  <SelectItem key={o.value} value={o.value}>
                    {o.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-2">
            <Label htmlFor="transfer-reason">
              {t("departments.transfer.reason")}
            </Label>
            <Textarea
              id="transfer-reason"
              rows={2}
              maxLength={1000}
              value={reason}
              onChange={(e) => setReason(e.target.value)}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="transfer-summary">
              {t("departments.transfer.summary")}
            </Label>
            <Textarea
              id="transfer-summary"
              rows={4}
              maxLength={4000}
              placeholder={t("departments.transfer.summary_placeholder")}
              value={summary}
              onChange={(e) => setSummary(e.target.value)}
            />
          </div>
          <label className="flex items-center gap-2 text-sm">
            <Checkbox
              checked={release}
              onCheckedChange={(v) => setRelease(v === true)}
            />
            {t("departments.transfer.release_assignee")}
          </label>
        </div>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose}>
            {t("form.cancel")}
          </Button>
          <Button
            type="button"
            disabled={!target || transfer.isPending}
            onClick={() =>
              transfer.mutate(
                {
                  department_uuid: target === ORG_WIDE ? null : target,
                  reason: reason.trim(),
                  context_summary: summary.trim(),
                  release_assignee: release,
                },
                {
                  onSuccess: () => {
                    setTarget("");
                    setReason("");
                    setSummary("");
                    setRelease(false);
                    onClose();
                  },
                },
              )
            }
          >
            {t("departments.transfer.submit")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/**
 * Department of a conversation in the inbox context pane (CF-215): who set
 * it (the customer in the widget, the AI with its confidence, an agent) and
 * the transfer history; agents who may assign hand it to another department
 * with its context.
 */
export function ConversationDepartmentPanel({
  conversationUuid,
}: {
  conversationUuid: string;
}) {
  const { t, locale } = useLocale();
  const { can } = usePermission();
  const canRead = can(permissions.tenant.departments.read);
  const canTransfer = canRead && can(permissions.tenant.conversations.assign);
  const query = useConversationDepartment(conversationUuid, canRead);
  const [open, setOpen] = useState(false);
  if (!canRead || !query.data) return null;
  const state = query.data;
  const last = state.transfers[0];
  return (
    <section>
      <h3 className="text-muted-foreground mb-2 flex items-center justify-between text-[11px] font-semibold tracking-wide uppercase">
        <span>{t("departments.panel.title")}</span>
        {canTransfer ? (
          <Button
            type="button"
            size="sm"
            variant="ghost"
            className="h-6 px-2 text-xs normal-case"
            onClick={() => setOpen(true)}
          >
            <ArrowRightLeft className="size-3" />
            {t("departments.panel.transfer")}
          </Button>
        ) : null}
      </h3>
      <p className="text-sm font-medium">
        {state.department?.name ?? t("departments.panel.organization_wide")}
      </p>
      {state.source ? (
        <p className="text-muted-foreground flex items-center gap-1 text-xs">
          {state.source === "ai" ? <Sparkles className="size-3" /> : null}
          {t(`departments.panel.source.${state.source}`)}
          {state.source === "ai" && state.confidence != null
            ? ` · %${Math.round(state.confidence * 100)}`
            : ""}
        </p>
      ) : null}
      {last && (last.reason || last.context_summary) ? (
        <div className="bg-muted/50 mt-2 space-y-1 rounded-md p-2 text-xs">
          {last.reason ? <p className="font-medium">{last.reason}</p> : null}
          {last.context_summary ? (
            <p className="text-muted-foreground whitespace-pre-wrap">
              {last.context_summary}
            </p>
          ) : null}
          <p className="text-muted-foreground">
            {(last.actor_name ||
              t(`departments.panel.actor.${last.actor_kind}`)) +
              " · " +
              datetime(last.created_at, "dd.MM HH:mm", locale)}
          </p>
        </div>
      ) : null}
      {canTransfer ? (
        <TransferDialog
          conversation={conversationUuid}
          current={state.department?.uuid}
          open={open}
          onClose={() => setOpen(false)}
        />
      ) : null}
    </section>
  );
}
