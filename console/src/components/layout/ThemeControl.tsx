"use client";

import { useEffect, useState } from "react";

type Theme = "ledger" | "vault" | "hc";

const themes: Array<{ value: Theme; label: string }> = [
    { value: "ledger", label: "Ledger" },
    { value: "vault", label: "Vault" },
    { value: "hc", label: "High contrast" },
];

export function ThemeControl() {
    const [theme, setTheme] = useState<Theme>("ledger");

    useEffect(() => {
        const stored = window.localStorage.getItem("ck-theme");
        const initial = themes.some((item) => item.value === stored) ? stored as Theme : "ledger";
        document.documentElement.dataset.theme = initial;
        const frame = window.requestAnimationFrame(() => setTheme(initial));
        return () => window.cancelAnimationFrame(frame);
    }, []);

    function applyTheme(nextTheme: Theme) {
        setTheme(nextTheme);
        document.documentElement.dataset.theme = nextTheme;
        window.localStorage.setItem("ck-theme", nextTheme);
    }

    return (
        <label className="flex min-h-11 items-center gap-2 border border-border px-3">
            <span className="ck-label hidden lg:inline">Theme</span>
            <select
                aria-label="Console theme"
                className="min-h-11 bg-background font-mono text-[11px] font-semibold uppercase tracking-[0.08em] text-foreground outline-none"
                onChange={(event) => applyTheme(event.target.value as Theme)}
                value={theme}
            >
                {themes.map((item) => (
                    <option key={item.value} value={item.value}>{item.label}</option>
                ))}
            </select>
        </label>
    );
}
