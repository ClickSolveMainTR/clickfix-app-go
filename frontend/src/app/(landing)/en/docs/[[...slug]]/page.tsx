import {
  renderMetadata,
  renderPage,
  generateStaticParams,
} from "@/features/docs/page";
export { generateStaticParams };
export const dynamicParams = false;
type Props = { params: Promise<{ slug?: string[] }> };
export async function generateMetadata({ params }: Props) {
  return renderMetadata({ params, locale: "en" });
}
export default async function Page({ params }: Props) {
  return renderPage({ params, locale: "en" });
}
