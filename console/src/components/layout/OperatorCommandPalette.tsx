"use client";

import { useEffect, useMemo, useState } from "react";
import { RefreshCw, Search } from "lucide-react";
import { usePathname } from "next/navigation";
import { useTranslations } from "next-intl";

import { useRouter } from "@/i18n/routing";
import type { DeploymentProfile } from "@/lib/config";
import {
    CommandDialog,
    CommandEmpty,
    CommandGroup,
    CommandInput,
    CommandItem,
    CommandList,
    CommandSeparator,
    CommandShortcut,
} from "@/components/ui/command";
import { getOperatorNavigation } from "./operator-navigation";
import { useOperatorRefresh } from "./OperatorRefreshProvider";

export function OperatorCommandPalette({ profile }: { profile: DeploymentProfile }) {
    const [open, setOpen] = useState(false);
    const [shortcut, setShortcut] = useState("Meta K");
    const router = useRouter();
    const pathname = usePathname();
    const t = useTranslations("Dashboard");
    const { isRefreshing, refreshSources } = useOperatorRefresh();
    const normalizedPath = pathname.replace(/^\/[a-z]{2}(?=\/|$)/, "") || "/";
    const navigation = useMemo(() => getOperatorNavigation(profile), [profile]);

    useEffect(() => {
        const isMac = /Mac|iPhone|iPad/.test(window.navigator.platform);
        const frame = window.requestAnimationFrame(() => setShortcut(isMac ? "Meta K" : "Ctrl K"));

        function onKeyDown(event: KeyboardEvent) {
            if (event.key.toLowerCase() !== "k" || (!event.metaKey && !event.ctrlKey)) return;
            event.preventDefault();
            setOpen((current) => !current);
        }

        window.addEventListener("keydown", onKeyDown);
        return () => {
            window.cancelAnimationFrame(frame);
            window.removeEventListener("keydown", onKeyDown);
        };
    }, []);

    function navigate(url: string) {
        setOpen(false);
        router.push(url as never);
    }

    function runRefresh() {
        setOpen(false);
        refreshSources();
    }

    return (
        <>
            <button
                aria-keyshortcuts="Meta+K Control+K"
                aria-label="Open command palette"
                className="ck-command-trigger"
                onClick={() => setOpen(true)}
                type="button"
            >
                <Search className="size-4" aria-hidden="true" />
                <span className="hidden xl:inline">Find</span>
                <kbd>{shortcut}</kbd>
            </button>

            <CommandDialog
                className="ck-command-dialog"
                description="Find an operator surface or run a local console action."
                onOpenChange={setOpen}
                open={open}
                title="Operator command palette"
            >
                <CommandInput placeholder="Find surface or action…" />
                <CommandList>
                    <CommandEmpty>No matching surface or action.</CommandEmpty>
                    {navigation.map((group) => {
                        const enabledItems = group.items.filter((item) => item.enabled);
                        if (enabledItems.length === 0) return null;

                        return (
                            <CommandGroup heading={group.label} key={group.label}>
                                {enabledItems.map((item) => {
                                    const title = t(item.titleKey);
                                    const current = normalizedPath === item.url;
                                    return (
                                        <CommandItem
                                            aria-current={current ? "page" : undefined}
                                            key={item.url}
                                            keywords={[item.keywords, item.tooltip]}
                                            onSelect={() => navigate(item.url)}
                                            value={title}
                                        >
                                            <item.icon aria-hidden="true" />
                                            <span>{title}</span>
                                            {current ? <CommandShortcut>Current</CommandShortcut> : null}
                                        </CommandItem>
                                    );
                                })}
                            </CommandGroup>
                        );
                    })}
                    <CommandSeparator />
                    <CommandGroup heading="Operator action">
                        <CommandItem
                            aria-busy={isRefreshing}
                            disabled={isRefreshing}
                            keywords={["reload retry observe fetch"]}
                            onSelect={runRefresh}
                            value="Refresh sources"
                        >
                            <RefreshCw aria-hidden="true" />
                            <span>{isRefreshing ? "Refreshing sources…" : "Refresh sources"}</span>
                            <CommandShortcut>{isRefreshing ? "Pending" : "R"}</CommandShortcut>
                        </CommandItem>
                    </CommandGroup>
                </CommandList>
                <div className="ck-command-footnote">
                    <span>Navigate and observe only</span>
                    <span>Esc closes</span>
                </div>
            </CommandDialog>
        </>
    );
}
