"use client";

import { Sparkles, Video, Wallet } from "lucide-react";
import type { ReactNode } from "react";

import { Skeleton } from "@/components/ui/skeleton";
import { formatCount, formatMinor } from "@/features/billing/lib/money";
import type { TenantWallet } from "@/features/billing/services/tenant-billing.service";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

function BalanceCard({
  icon,
  title,
  value,
  unit,
  hint,
  tone = "default",
}: {
  icon: ReactNode;
  title: string;
  value: string;
  unit?: string;
  hint: ReactNode;
  tone?: "default" | "warn" | "danger";
}) {
  return (
    <div
      className={cn(
        "bg-card flex flex-col gap-3 rounded-xl border p-4",
        tone === "warn" && "border-amber-500/40",
        tone === "danger" && "border-destructive/50",
      )}
    >
      <div className="text-muted-foreground flex items-center gap-2 text-sm font-medium">
        <span className="bg-muted text-foreground flex size-7 items-center justify-center rounded-md [&>svg]:size-4">
          {icon}
        </span>
        {title}
      </div>
      <p className="flex items-baseline gap-1.5">
        <span className="text-3xl font-semibold tracking-tight tabular-nums">
          {value}
        </span>
        {unit ? (
          <span className="text-muted-foreground text-sm">{unit}</span>
        ) : null}
      </p>
      <div
        className={cn(
          "text-xs leading-relaxed",
          tone === "danger"
            ? "text-destructive"
            : tone === "warn"
              ? "text-amber-700 dark:text-amber-400"
              : "text-muted-foreground",
        )}
      >
        {hint}
      </div>
    </div>
  );
}

/**
 * AI credit, video minute and (when there is any) TRY balance. The AI card
 * turns amber under the alert level and red once the balance is spent
 * (`ai_action` = stop_and_handoff: AI replies stop, conversations go to
 * the team).
 */
export function WalletBalances({
  wallet,
  isLoading,
  alertLevel,
}: {
  wallet: TenantWallet | undefined;
  isLoading: boolean;
  /** Effective low balance alert level (saved or the plan default). */
  alertLevel: number | null;
}) {
  const { t, locale } = useLocale();
  if (isLoading || !wallet)
    return (
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        <Skeleton className="h-36 rounded-xl" />
        <Skeleton className="h-36 rounded-xl" />
      </div>
    );

  const empty = wallet.ai_action === "stop_and_handoff";
  const low = !empty && alertLevel !== null && wallet.ai_credits < alertLevel;
  return (
    <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
      <BalanceCard
        icon={<Sparkles />}
        title={t("wallet.balance.ai.title")}
        value={formatCount(wallet.ai_credits, locale)}
        unit={t("wallet.balance.ai.unit")}
        tone={empty ? "danger" : low ? "warn" : "default"}
        hint={
          empty ? (
            t("wallet.balance.ai.empty")
          ) : (
            <>
              {low ? (
                <span className="block font-medium">
                  {t("wallet.balance.ai.low", {
                    threshold: formatCount(alertLevel ?? 0, locale),
                  })}
                </span>
              ) : null}
              <span className={cn(low && "text-muted-foreground mt-1 block")}>
                {t("wallet.balance.ai.hint")}
              </span>
            </>
          )
        }
      />
      <BalanceCard
        icon={<Video />}
        title={t("wallet.balance.video.title")}
        value={formatCount(wallet.video_minutes, locale)}
        unit={t("wallet.balance.video.unit")}
        hint={t("wallet.balance.video.hint")}
      />
      {wallet.try_minor > 0 ? (
        <BalanceCard
          icon={<Wallet />}
          title={t("wallet.balance.try.title")}
          value={formatMinor(wallet.try_minor, "TRY", locale)}
          hint={t("wallet.balance.try.hint")}
        />
      ) : null}
    </div>
  );
}
