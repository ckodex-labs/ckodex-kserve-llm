import type { Metadata } from "next";
import { Geist, Instrument_Serif, JetBrains_Mono } from "next/font/google";
import "../globals.css";
import { TooltipProvider } from "@/components/ui/tooltip";
import { SidebarProvider } from "@/components/ui/sidebar";
import { AppSidebar } from "@/components/layout/AppSidebar";
import { AISidePanel } from "@/components/features/AISidePanel";
import { OperatorRefreshProvider } from "@/components/layout/OperatorRefreshProvider";
import { NextIntlClientProvider } from 'next-intl';
import { getMessages } from 'next-intl/server';
import { notFound } from 'next/navigation';
import { routing } from '@/i18n/routing';
import { getDeploymentProfile } from "@/lib/kserve";

const geistSans = Geist({
  variable: "--font-geist-sans",
  subsets: ["latin"],
});

const jetbrainsMono = JetBrains_Mono({
  variable: "--font-jetbrains-mono",
  subsets: ["latin"],
});

const instrumentSerif = Instrument_Serif({
  variable: "--font-instrument-serif",
  subsets: ["latin"],
  weight: "400",
});

export const metadata: Metadata = {
  title: "CKodex KServe LLM Operator",
  description: "Observed Kubernetes reconciliation state for CKodex model-serving resources.",
};

export default async function RootLayout({
  children,
  params
}: {
  children: React.ReactNode;
  params: Promise<{ locale: string }>
}) {
  const { locale } = await params;

  if (!routing.locales.some((supportedLocale) => supportedLocale === locale)) {
    notFound();
  }

  const messages = await getMessages();
  const profile = await getDeploymentProfile();

  return (
    <html
      lang={locale}
      data-theme="ledger"
      className={`${geistSans.variable} ${jetbrainsMono.variable} ${instrumentSerif.variable} h-full antialiased`}
    >
      <body className="h-full bg-background text-foreground font-sans">
        <a className="ck-skip" href="#ck-main">Skip to content</a>
        <NextIntlClientProvider messages={messages}>
          <TooltipProvider>
            <OperatorRefreshProvider>
              <SidebarProvider defaultOpen={true}>
                <AppSidebar profile={profile} />
                {children}
                <AISidePanel />
              </SidebarProvider>
            </OperatorRefreshProvider>
          </TooltipProvider>
        </NextIntlClientProvider>
      </body>
    </html>
  );
}
