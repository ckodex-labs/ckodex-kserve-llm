"use client";

import type { ComponentProps, ReactNode } from "react";

import { cn } from "@/lib/utils";

export type ConversationEmptyStateProps = ComponentProps<"div"> & {
    title?: string;
    description?: string;
    icon?: ReactNode;
};

export function ConversationEmptyState({
    className,
    title = "No messages yet",
    description = "Start a conversation to see messages here.",
    icon,
    children,
    ...props
}: ConversationEmptyStateProps) {
    return (
        <div className={cn("flex min-h-64 flex-col items-center justify-center gap-3 p-8 text-center", className)} {...props}>
            {children ?? (
                <>
                    {icon ? <div className="text-muted-foreground">{icon}</div> : null}
                    <div>
                        <h3 className="text-sm font-semibold">{title}</h3>
                        {description ? <p className="mt-2 text-sm leading-6 text-muted-foreground">{description}</p> : null}
                    </div>
                </>
            )}
        </div>
    );
}
