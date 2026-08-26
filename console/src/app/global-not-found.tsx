import type { Metadata } from "next";
import Link from "next/link";
import "./globals.css";

export const metadata: Metadata = {
    title: "Operator surface unavailable · CKodex",
    description: "The requested CKodex operator route does not resolve.",
};

export default function GlobalNotFound() {
    return (
        <html lang="en" data-theme="ledger">
            <body className="ck-global-recovery">
                <main>
                    <p className="ck-label">Route availability</p>
                    <div className="ck-route-state__mark mt-6" aria-hidden="true">○</div>
                    <h1>Operator surface unavailable</h1>
                    <p>The requested URL does not resolve to an operator route. No source or readiness observation is asserted.</p>
                    <Link href="/en">Reconciliation overview</Link>
                    <p className="ck-invariant">mode changes deployment, not governance semantics</p>
                </main>
            </body>
        </html>
    );
}
