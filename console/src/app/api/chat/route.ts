import { createOpenAI } from "@ai-sdk/openai";
import { streamText, type ModelMessage } from "ai";

import { resolveAiGatewayBaseUrl } from "@/lib/ai-gateway-url";

export const maxDuration = 30;

const maxMessages = 32;
const maxContentLength = 12_000;
const defaultGatewayTimeoutMs = 28_000;

function gatewayTimeoutMs(): number {
    const configured = Number(process.env.CKODEX_AI_GATEWAY_TIMEOUT_MS);
    if (!Number.isFinite(configured)) return defaultGatewayTimeoutMs;
    return Math.min(28_000, Math.max(1_000, Math.trunc(configured)));
}

interface GatewayConfig {
    baseURL: string;
    apiKey: string;
    model: string;
}

function getGatewayConfig(): GatewayConfig | null {
    const allowInsecure = process.env.CKODEX_AI_GATEWAY_ALLOW_INSECURE === "true";
    const baseURL = resolveAiGatewayBaseUrl(process.env.CKODEX_AI_GATEWAY_URL, allowInsecure);
    const apiKey = process.env.CKODEX_AI_GATEWAY_API_KEY;
    const model = process.env.CKODEX_AI_MODEL;
    if (!baseURL || !apiKey || !model) return null;
    return { baseURL: baseURL.toString(), apiKey, model };
}

function parseMessages(value: unknown): ModelMessage[] | null {
    if (!Array.isArray(value) || value.length === 0 || value.length > maxMessages) return null;

    const messages: ModelMessage[] = [];
    for (const item of value) {
        if (typeof item !== "object" || item === null) return null;
        const { role, content } = item as { role?: unknown; content?: unknown };
        if ((role !== "user" && role !== "assistant") || typeof content !== "string") return null;
        if (content.length === 0 || content.length > maxContentLength) return null;
        messages.push({ role, content });
    }
    return messages;
}

export async function GET() {
    const config = getGatewayConfig();
    return Response.json({
        configured: config !== null,
        model: config?.model ?? null,
        mode: "advisory",
    }, {
        headers: { "Cache-Control": "no-store" },
    });
}

export async function POST(req: Request) {
    const config = getGatewayConfig();
    if (!config) {
        return Response.json(
            { error: "Operator assistant is not configured." },
            { status: 503 },
        );
    }

    const contentLength = Number(req.headers.get("content-length") || "0");
    if (contentLength > 400_000) {
        return Response.json({ error: "Request is too large." }, { status: 413 });
    }

    try {
        const body = await req.json() as { messages?: unknown };
        const messages = parseMessages(body.messages);
        if (!messages) {
            return Response.json({ error: "Invalid message payload." }, { status: 400 });
        }

        const openai = createOpenAI({ baseURL: config.baseURL, apiKey: config.apiKey });
        const timeoutMs = gatewayTimeoutMs();
        const result = streamText({
            model: openai(config.model),
            system: [
                "You are the advisory assistant for the CKODEX KServe LLM Operator console.",
                "Be technical, concise, and explicit about uncertainty.",
                "You have no cluster tools and cannot execute mutations.",
                "Do not claim live cluster state unless it is present in the user's message.",
            ].join(" "),
            messages,
            abortSignal: req.signal,
            maxRetries: 1,
            timeout: { totalMs: timeoutMs, chunkMs: Math.min(10_000, timeoutMs) },
            onAbort: () => console.warn("AI_GATEWAY_STREAM_ABORTED"),
            onError: ({ error }) => console.error(
                "AI_GATEWAY_STREAM_ERROR",
                error instanceof Error ? error.name : "UnknownError",
            ),
        });

        return result.toTextStreamResponse({
            headers: {
                "Cache-Control": "no-store",
                "X-Content-Type-Options": "nosniff",
            },
        });
    } catch (error) {
        console.error("AI_GATEWAY_ERROR", error);
        return Response.json({ error: "The configured assistant gateway did not respond." }, { status: 502 });
    }
}
