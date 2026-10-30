import type { Metadata, Viewport } from "next";
import { Geist, Geist_Mono, Oxanium, Space_Grotesk } from "next/font/google";
import Script from "next/script";

import { brand } from "@/config/brand";
import { SignupConversion } from "@/features/landing/components/tracking/signup-conversion";
import { AppProviders } from "@/providers/app-providers";

import "./globals.css";

const geistSans = Geist({
  variable: "--font-geist-sans",
  subsets: ["latin", "latin-ext"],
});

/** Code and tabular figures only: not preloaded, so it never competes with
 * the first paint (CF-144). */
const geistMono = Geist_Mono({
  variable: "--font-geist-mono",
  subsets: ["latin", "latin-ext"],
  preload: false,
});

/** Display face for headings and the typeset wordmark (docs/brand.md). */
const spaceGrotesk = Space_Grotesk({
  variable: "--font-space-grotesk",
  subsets: ["latin", "latin-ext"],
});

/** Accent face for eyebrows and big numerals; never body text. */
const oxanium = Oxanium({
  variable: "--font-oxanium",
  subsets: ["latin", "latin-ext"],
  weight: ["500", "600"],
  preload: false,
});

/** Absolute base for canonical / Open Graph URLs; localhost in dev. */
function siteOrigin(): URL {
  const raw = process.env.SITE_URL || process.env.AUTH_URL || brand.siteUrl;
  try {
    return new URL(raw);
  } catch {
    return new URL(brand.siteUrl);
  }
}

export const metadata: Metadata = {
  metadataBase: siteOrigin(),
  alternates: { canonical: "/" },
  openGraph: {
    type: "website",
    url: "/",
    siteName: brand.productName,
    title: brand.productName,
    description: brand.tagline,
    locale: "tr_TR",
  },
  title: {
    default: brand.productName,
    template: `%s · ${brand.name}`,
  },
  description: brand.tagline,
  applicationName: brand.productName,
};

export const viewport: Viewport = {
  themeColor: [
    { media: "(prefers-color-scheme: light)", color: brand.colors.paper },
    { media: "(prefers-color-scheme: dark)", color: brand.colors.ink },
  ],
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="tr" suppressHydrationWarning className="h-full">
      <body
        className={`${geistSans.variable} ${geistMono.variable} ${spaceGrotesk.variable} ${oxanium.variable} min-h-full font-sans antialiased`}
      >
        {/* Runtime browser config (Sentry DSN); must run before hydration. */}
        <Script src="/api/client-env" strategy="beforeInteractive" />
        <AppProviders>{children}</AppProviders>
        <SignupConversion />
      </body>
    </html>
  );
}
