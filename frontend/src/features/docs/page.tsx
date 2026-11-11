import {
  DocsBody,
  DocsDescription,
  DocsPage,
  DocsTitle,
} from "fumadocs-ui/page";
import defaultComponents from "fumadocs-ui/mdx";
import { notFound } from "next/navigation";
import type { Metadata } from "next";

import { brand } from "@/config/brand";
import { marketingMetadata } from "@/features/landing";
import { OpenAPIPage } from "@/features/docs/components/api-page";
import { DocsPageActions } from "@/features/docs/components/page-actions";
import { markdownPath, pageUrl } from "@/features/docs/lib/markdown";
import { openapi } from "@/lib/docs/openapi";
import { source } from "@/lib/docs/source";

export function generateStaticParams() {
  return source.getPages("tr").map((page) => ({ slug: page.slugs }));
}
type Props = { params: Promise<{ slug?: string[] }>; locale: "tr" | "en" };
export async function renderMetadata({
  params,
  locale,
}: Props): Promise<Metadata> {
  const page = source.getPage((await params).slug, locale);
  if (!page) notFound();
  const metadata = marketingMetadata({
    locale,
    path: `/docs${page.slugs.length ? `/${page.slugs.join("/")}` : ""}`,
    title: page.data.title,
    description:
      page.data.description ??
      (locale === "tr" ? "ClickFix dokümantasyonu" : "ClickFix documentation"),
    type: "article",
  });
  return {
    ...metadata,
    openGraph: {
      ...metadata.openGraph,
      images: [
        {
          url: `/docs/og/${page.slugs.join("/") || "index"}${locale === "en" ? "?lang=en" : ""}`,
          width: 1200,
          height: 630,
        },
      ],
    },
  };
}
export async function renderPage({ params, locale }: Props) {
  const page = source.getPage((await params).slug, locale);
  if (!page) notFound();
  const Content = page.data.body;
  const preloaded = page.data._openapi
    ? await openapi.preloadOpenAPIPage(page)
    : {};
  const lastModified = (page.data as { lastModified?: Date }).lastModified;
  return (
    <DocsPage
      toc={page.data.toc}
      full={page.data.full}
      lastUpdate={lastModified}
      breadcrumb={{
        includeRoot: { url: locale === "en" ? "/docs?lang=en" : "/docs" },
        includeSeparator: true,
      }}
    >
      <DocsTitle>{page.data.title}</DocsTitle>
      <DocsDescription>{page.data.description}</DocsDescription>
      <DocsPageActions
        locale={locale}
        title={page.data.title}
        markdownUrl={markdownPath(page, locale)}
        pageUrl={pageUrl(page)}
        feedbackEmail={brand.supportEmail}
      />
      <DocsBody>
        <Content
          components={{
            ...defaultComponents,
            OpenAPIPage: (props) => <OpenAPIPage {...preloaded} {...props} />,
          }}
        />
      </DocsBody>
    </DocsPage>
  );
}
