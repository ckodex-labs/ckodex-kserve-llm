"use client";

import { useState } from "react";
import {
    Sheet,
    SheetContent,
    SheetHeader,
    SheetTitle,
    SheetDescription
} from "@/components/ui/sheet";
import { AIOrb } from "./AIOrb";
import { ChatInterface } from "./ChatInterface";

export function AISidePanel() {
    const [open, setOpen] = useState(false);

    return (
        <>
            <AIOrb onClick={() => setOpen(true)} isOpen={open} />

            <Sheet open={open} onOpenChange={setOpen}>
                <SheetContent className="w-[min(100vw,500px)] border-l border-border bg-background p-0 shadow-none">
                    <SheetHeader className="sr-only">
                        <SheetTitle>AI Assistant</SheetTitle>
                        <SheetDescription>
                            Query operator data and inspect returned evidence.
                        </SheetDescription>
                    </SheetHeader>

                    <div className="h-full flex flex-col">
                        <ChatInterface active={open} />
                    </div>
                </SheetContent>
            </Sheet>
        </>
    );
}
