import { DocsLayout } from "fumadocs-ui/layouts/docs";
import type { ReactNode } from "react";

import { DocsProvider } from "@/features/docs/components/docs-provider";
import { DocsToolbar } from "@/features/docs/components/docs-toolbar";
import { MarketingShell } from "@/features/landing/components/marketing-shell";
import { isSelfSignupOpen } from "@/features/landing/lib/signup";
import { source } from "@/lib/docs/source";

/**
 * Docs shell (CF-208): the marketing site's fixed top bar (logo, product,
 * pricing, docs, sign in) above the fumadocs layout; the sidebar carries the
 * language and colour-mode switches and the grouped page tree.
 */
export async function DocsSiteLayout({
  children,
  locale,
}: {
  children: ReactNode;
  locale: "tr" | "en";
}) {
  return (
    <MarketingShell
      locale={locale}
      signupOpen={await isSelfSignupOpen()}
      navSolid
    >
      <div className="clickfix-docs pt-16">
        <DocsProvider locale={locale}>
          <DocsLayout
            tree={source.getPageTree(locale)}
            nav={{ title: locale === "tr" ? "Dokümantasyon" : "Documentation" }}
            themeSwitch={{ enabled: false }}
            sidebar={{ banner: <DocsToolbar locale={locale} /> }}
          >
            {children}
          </DocsLayout>
        </DocsProvider>
      </div>
    </MarketingShell>
  );
}
