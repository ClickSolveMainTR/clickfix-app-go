import { renderMetadata, renderPage } from "@/features/landing/pages/home";

export const revalidate = 300;

export async function generateMetadata() {
  return renderMetadata({ searchParams: Promise.resolve({ lang: "en" }) });
}
export default async function Page() {
  return renderPage({ searchParams: Promise.resolve({ lang: "en" }) });
}
