import { ogImage } from "@/features/landing/lib/og";
import { source } from "@/lib/docs/source";

export const dynamic = "force-static";
export const dynamicParams = false;
export function generateStaticParams() {
  return source
    .getPages("tr")
    .map((page) => ({ slug: page.slugs.length ? page.slugs : ["index"] }));
}
export async function GET(
  _request: Request,
  { params }: { params: Promise<{ slug: string[] }> },
) {
  const { slug } = await params;
  const page = source.getPage(slug[0] === "index" ? [] : slug);
  return ogImage({
    eyebrow: "ClickFix · Dokümantasyon",
    title: page?.data.title ?? "ClickFix",
  });
}
