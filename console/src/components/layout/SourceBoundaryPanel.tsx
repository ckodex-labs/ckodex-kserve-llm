import type { SourceSnapshot } from "@/lib/source";
import { sourceClaim } from "@/lib/source";

interface SourceBoundaryPanelProps {
    label: string;
    title: string;
    snapshot: Pick<SourceSnapshot<unknown>, "state" | "source" | "detail">;
}

export function SourceBoundaryPanel({ label, title, snapshot }: SourceBoundaryPanelProps) {
    return (
        <section className="ck-quiet p-6" aria-labelledby={`${label}-boundary-title`}>
            <div className="flex flex-col justify-between gap-4 sm:flex-row sm:items-start">
                <div>
                    <p className="ck-label">Source boundary</p>
                    <h2 className="mt-3 text-2xl font-semibold" id={`${label}-boundary-title`}>{title}</h2>
                </div>
                <span className="ck-claim ck-claim--muted">{sourceClaim(label, snapshot.state)}</span>
            </div>
            <p className="mt-4 max-w-[72ch] text-sm leading-6 text-muted-foreground">{snapshot.detail}</p>
            <p className="mt-4 break-words border-t border-border pt-4 font-mono text-xs text-muted-foreground">{snapshot.source}</p>
        </section>
    );
}
