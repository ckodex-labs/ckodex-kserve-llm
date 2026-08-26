"use client";

import { Command } from "lucide-react";
import { cn } from "@/lib/utils";

interface AIOrbProps {
    onClick: () => void;
    isOpen?: boolean;
    className?: string;
}

export function AIOrb({ onClick, isOpen, className }: AIOrbProps) {
    return (
        <button
            type="button"
            onClick={onClick}
            aria-label={isOpen ? "Close operator assistant" : "Open operator assistant"}
            aria-expanded={isOpen}
            className={cn(
                "ck-assistant-trigger fixed z-50 flex size-12 items-center justify-center border border-primary bg-primary text-primary-foreground",
                "transition-colors hover:bg-background hover:text-foreground",
                isOpen && "bg-background text-foreground",
                className
            )}
        >
            <Command className="size-5" aria-hidden="true" />
        </button>
    );
}
