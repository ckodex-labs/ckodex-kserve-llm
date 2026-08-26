"use client";

import { useEffect } from "react";
import "./globals.css";

export default function GlobalError({
    error,
    unstable_retry,
}: {
    error: Error & { digest?: string };
    unstable_retry: () => void;
}) {
    useEffect(() => {
        console.error("OPERATOR_ROOT_RENDER_ERROR", error);
    }, [error]);

    return (
        <html lang="en" data-theme="ledger">
            <body className="ck-global-recovery">
                <main>
                    <p className="ck-label">Console recovery</p>
                    <div className="ck-route-state__mark mt-6" aria-hidden="true">⊭</div>
                    <h1>Console shell interrupted</h1>
                    <p>The root application shell did not render. Retry the document before relying on any operator state.</p>
                    {error.digest ? <code>Runtime reference · {error.digest}</code> : null}
                    <button onClick={unstable_retry} type="button">Retry console</button>
                    <p className="ck-invariant">mode changes deployment, not governance semantics</p>
                </main>
            </body>
        </html>
    );
}
