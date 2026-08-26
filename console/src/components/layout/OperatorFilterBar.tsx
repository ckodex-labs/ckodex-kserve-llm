"use client";

import type { ReactNode } from "react";

import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

interface OperatorFilterBarProps {
    children: ReactNode;
    className?: string;
    fieldsClassName?: string;
    hasActiveFilters: boolean;
    label: string;
    onReset: () => void;
    status: string;
}

export function OperatorFilterBar({
    children,
    className,
    fieldsClassName,
    hasActiveFilters,
    label,
    onReset,
    status,
}: OperatorFilterBarProps) {
    return (
        <section aria-label={label} className={cn("border border-border", className)}>
            <div className={cn("grid gap-3 p-5", fieldsClassName)}>
                {children}
            </div>
            <div className="flex min-h-11 flex-col gap-2 border-t border-border px-5 py-2 sm:flex-row sm:items-center sm:justify-between">
                <p aria-live="polite" className="font-mono text-[11px] uppercase tracking-[0.08em] text-muted-foreground" role="status">
                    {status}
                </p>
                <Button
                    disabled={!hasActiveFilters}
                    onClick={onReset}
                    size="sm"
                    type="button"
                    variant="ghost"
                >
                    Reset filters
                </Button>
            </div>
        </section>
    );
}
