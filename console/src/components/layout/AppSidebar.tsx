"use client";

import { useEffect, useRef } from "react";
import { Link } from "@/i18n/routing";
import { usePathname } from "next/navigation";
import { useTranslations } from "next-intl";
import {
    Command,
} from "lucide-react";
import {
    Sidebar,
    SidebarContent,
    SidebarFooter,
    SidebarHeader,
    SidebarMenu,
    SidebarMenuButton,
    SidebarMenuItem,
    SidebarGroup,
    SidebarGroupLabel,
    SidebarGroupContent,
    SidebarRail,
} from "@/components/ui/sidebar";
import type { DeploymentProfile } from "@/lib/config";
import { cn } from "@/lib/utils";
import { getOperatorNavigation } from "./operator-navigation";

interface AppSidebarProps {
    profile: DeploymentProfile;
}

export function AppSidebar({ profile }: AppSidebarProps) {
    const pathname = usePathname();
    const navigationRef = useRef<HTMLElement>(null);
    const t = useTranslations("Dashboard");
    const normalizedPath = pathname.replace(/^\/[a-z]{2}(?=\/|$)/, "") || "/";

    const navGroups = getOperatorNavigation(profile);

    useEffect(() => {
        if (!window.matchMedia("(max-width: 900px)").matches) return;
        const frame = window.requestAnimationFrame(() => {
            navigationRef.current
                ?.querySelector<HTMLElement>('[aria-current="page"]')
                ?.scrollIntoView({ block: "nearest", inline: "center" });
        });
        return () => window.cancelAnimationFrame(frame);
    }, [pathname]);

    return (
        <Sidebar variant="sidebar" collapsible="icon" mobileMode="inline" className="ck-operator-navigation border-r border-sidebar-border bg-sidebar">
            <SidebarHeader className="border-b border-sidebar-border p-4">
                <div className="flex min-h-14 items-center border border-sidebar-border bg-background p-2">
                    <div className="flex size-9 shrink-0 items-center justify-center border border-sidebar-border text-sidebar-foreground">
                        <Command className="size-4" aria-hidden="true" />
                    </div>
                    <div className="ml-2 min-w-0 flex-1 text-left group-data-[collapsible=icon]:hidden">
                        <span className="block truncate text-sm font-semibold">{profile.latticeName}</span>
                        <span className="mt-1 block truncate font-mono text-[11px] uppercase tracking-[0.12em] text-muted-foreground">
                            {profile.environment} · observe
                        </span>
                    </div>
                </div>
            </SidebarHeader>

            <SidebarContent className="ck-operator-navigation__content px-2">
                <nav aria-label="Primary navigation" className="ck-primary-nav" ref={navigationRef}>
                {navGroups.map((group) => {
                    const enabledItems = group.items.filter((item) => item.enabled);
                    if (enabledItems.length === 0) return null;

                    return (
                        <SidebarGroup key={group.label} className="ck-primary-nav__group mt-3">
                            <SidebarGroupLabel className="ck-primary-nav__label px-3 font-mono text-[11px] font-semibold uppercase tracking-[0.14em] text-muted-foreground">
                                {group.label}
                            </SidebarGroupLabel>
                            <SidebarGroupContent className="ck-primary-nav__content mt-1">
                                <SidebarMenu className="ck-primary-nav__items gap-1">
                                    {enabledItems.map((item) => {
                                        const title = t(item.titleKey);
                                        const current = normalizedPath === item.url;
                                        return (
                                        <SidebarMenuItem className="ck-primary-nav__item" key={item.url}>
                                            <SidebarMenuButton
                                                aria-current={current ? "page" : undefined}
                                                isActive={current}
                                                tooltip={item.tooltip}
                                                className={cn(
                                                    "ck-primary-nav__link",
                                                    "min-h-11 border border-transparent px-3",
                                                    "hover:border-sidebar-border hover:bg-background",
                                                    "data-[active=true]:border-sidebar-border data-[active=true]:bg-background data-[active=true]:font-semibold"
                                                )}
                                                render={<Link href={item.url as never} />}
                                            >
                                                <item.icon className="size-4 shrink-0" aria-hidden="true" />
                                                <span className="ml-2 text-sm">{title}</span>
                                            </SidebarMenuButton>
                                        </SidebarMenuItem>
                                        );
                                    })}
                                </SidebarMenu>
                            </SidebarGroupContent>
                        </SidebarGroup>
                    );
                })}
                </nav>
            </SidebarContent>

            <SidebarFooter className="border-t border-sidebar-border p-3">
                <p className="px-3 py-2 font-mono text-[11px] uppercase tracking-[0.12em] text-muted-foreground group-data-[collapsible=icon]:hidden">
                    ○ Profile configured locally
                </p>
            </SidebarFooter>
            <SidebarRail />
        </Sidebar>
    );
}
