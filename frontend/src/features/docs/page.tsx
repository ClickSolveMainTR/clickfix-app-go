import {
  DocsBody,
  DocsDescription,
  DocsPage,
  DocsTitle,
} from "fumadocs-ui/page";
import defaultComponents from "fumadocs-ui/mdx";
import { notFound } from "next/navigation";
import type { Metadata } from "next";

import { marketingMetadata } from "@/features/landing";
import { OpenAPIPage } from "@/features/docs/components/api-page";
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
    description: page.data.description ?? "ClickFix dokümantasyonu",
    type: "article",
  });
  return {
    ...metadata,
    openGraph: {
      ...metadata.openGraph,
      images: [
        {
          url: `/docs/og/${page.slugs.join("/") || "index"}`,
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
  return (
    <DocsPage toc={page.data.toc} full={page.data.full}>
      <DocsTitle>{page.data.title}</DocsTitle>
      <DocsDescription>{page.data.description}</DocsDescription>
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
