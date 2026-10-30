import { DocsLayout } from "fumadocs-ui/layouts/docs";
import type { ReactNode } from "react";

import { DocsProvider } from "@/features/docs/components/docs-provider";
import { MarketingShell } from "@/features/landing/components/marketing-shell";
import { isSelfSignupOpen } from "@/features/landing/lib/signup";
import { source } from "@/lib/docs/source";

export async function DocsSiteLayout({
  children,
  locale,
}: {
  children: ReactNode;
  locale: "tr" | "en";
}) {
  return (
    <MarketingShell locale={locale} signupOpen={await isSelfSignupOpen()}>
      <div className="clickfix-docs pt-16">
        <DocsProvider locale={locale}>
          <DocsLayout
            tree={source.getPageTree(locale)}
            nav={{ title: locale === "tr" ? "Dokümantasyon" : "Documentation" }}
            themeSwitch={{ enabled: false }}
          >
            {children}
          </DocsLayout>
        </DocsProvider>
      </div>
    </MarketingShell>
  );
}
