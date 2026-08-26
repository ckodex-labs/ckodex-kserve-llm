export function resolveAiGatewayBaseUrl(value: string | undefined, allowInsecure: boolean) {
    if (!value) return null;
    try {
        const url = new URL(value);
        const loopback = url.hostname === "127.0.0.1" || url.hostname === "localhost" || url.hostname === "[::1]";
        if (url.username || url.password || url.search || url.hash) return null;
        if (url.protocol !== "https:" && !(url.protocol === "http:" && (loopback || allowInsecure))) return null;
        if (url.protocol !== "http:" && url.protocol !== "https:") return null;
        url.pathname = url.pathname.replace(/\/+$/, "");
        return url;
    } catch {
        return null;
    }
}
