export const dynamic = "force-dynamic";

const headers = { "Cache-Control": "no-store" };

export function GET() {
    return Response.json({ status: "ok", scope: "console-process" }, { headers });
}

export function HEAD() {
    return new Response(null, { status: 200, headers });
}
