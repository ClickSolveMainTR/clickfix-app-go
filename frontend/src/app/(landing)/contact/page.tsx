import { renderMetadata, renderPage } from "@/features/landing/pages/contact";

export const revalidate = 300;

export async function generateMetadata() {
  return renderMetadata({ searchParams: Promise.resolve({}) });
}
export default async function Page() {
  return renderPage({ searchParams: Promise.resolve({}) });
}
