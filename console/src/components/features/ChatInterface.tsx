"use client";

import { useEffect, useRef, useState } from "react";
import { Command, SendIcon } from "lucide-react";
import { nanoid } from "nanoid";

import { ConversationEmptyState } from "@/components/ai-elements/conversation";
import { Message, MessageContent, MessageResponse } from "@/components/ai-elements/message";
import { Button } from "@/components/ui/button";
import { ScrollArea } from "@/components/ui/scroll-area";
import {
    assistantConfigurationTimeoutMs,
    assistantFailureDetail,
    assistantResponseTimeoutMs,
    createAssistantRequest,
    type AssistantRequest,
} from "@/lib/assistant-request";

type AssistantStatus = "checking" | "configured" | "unavailable";
type ChatMessage = {
    id: string;
    role: "user" | "assistant" | "error" | "notice";
    content: string;
};

export function ChatInterface({ active }: { active: boolean }) {
    const [messages, setMessages] = useState<ChatMessage[]>([]);
    const [input, setInput] = useState("");
    const [isLoading, setIsLoading] = useState(false);
    const [status, setStatus] = useState<AssistantStatus>("checking");
    const [statusDetail, setStatusDetail] = useState<string | null>(null);
    const [model, setModel] = useState<string | null>(null);
    const scrollRef = useRef<HTMLDivElement>(null);
    const activeRequest = useRef<AssistantRequest | null>(null);

    useEffect(() => {
        if (!active) return;
        let cancelled = false;
        const request = createAssistantRequest(assistantConfigurationTimeoutMs);
        setStatus("checking");
        setStatusDetail(null);

        fetch("/api/chat", { method: "GET", signal: request.signal })
            .then(async (response) => response.ok ? response.json() as Promise<{ configured?: boolean; model?: string | null }> : null)
            .then((configuration) => {
                if (cancelled) return;
                setStatus(configuration?.configured ? "configured" : "unavailable");
                setModel(configuration?.model ?? null);
                setStatusDetail(configuration?.configured ? null : "The advisory gateway configuration is incomplete.");
            })
            .catch((error) => {
                if (cancelled) return;
                const failure = request.classifyFailure(error);
                if (failure === "lifecycle-cancelled") return;
                setStatus("unavailable");
                setStatusDetail(assistantFailureDetail(failure, assistantConfigurationTimeoutMs));
            });
        return () => {
            cancelled = true;
            request.cancel("lifecycle");
        };
    }, [active]);

    useEffect(() => {
        if (!active) activeRequest.current?.cancel("lifecycle");
        return () => activeRequest.current?.cancel("lifecycle");
    }, [active]);

    useEffect(() => {
        const scrollContainer = scrollRef.current?.querySelector("[data-radix-scroll-area-viewport]");
        if (scrollContainer) scrollContainer.scrollTop = scrollContainer.scrollHeight;
    }, [messages, isLoading]);

    async function handleSubmit(event?: React.FormEvent) {
        event?.preventDefault();
        const content = input.trim();
        if (!content || isLoading || status !== "configured") return;

        const userMessage: ChatMessage = { id: nanoid(), role: "user", content };
        const priorMessages = messages
            .filter((message): message is ChatMessage & { role: "user" | "assistant" } => (
                message.role === "user" || message.role === "assistant"
            ))
            .slice(-31);
        const conversation = [
            ...priorMessages,
            userMessage,
        ];
        setMessages((current) => [...current, userMessage]);
        setInput("");
        setIsLoading(true);
        const request = createAssistantRequest(assistantResponseTimeoutMs);
        activeRequest.current = request;

        try {
            const response = await fetch("/api/chat", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({
                    messages: conversation.map(({ role, content: messageContent }) => ({ role, content: messageContent })),
                }),
                signal: request.signal,
            });

            if (!response.ok || !response.body) {
                const payload = await response.json().catch(() => null) as { error?: string } | null;
                throw new Error(payload?.error || "Assistant request failed.");
            }

            const assistantId = nanoid();
            setMessages((current) => [...current, { id: assistantId, role: "assistant", content: "" }]);
            const reader = response.body.getReader();
            const decoder = new TextDecoder();
            let accumulated = "";

            while (true) {
                const { value, done } = await reader.read();
                if (done) break;
                accumulated += decoder.decode(value, { stream: true });
                setMessages((current) => current.map((message) => (
                    message.id === assistantId ? { ...message, content: accumulated } : message
                )));
            }

            accumulated += decoder.decode();
            setMessages((current) => current.map((message) => (
                message.id === assistantId ? { ...message, content: accumulated || "No response content returned." } : message
            )));
        } catch (error) {
            const failure = request.classifyFailure(error);
            if (failure !== "lifecycle-cancelled") {
                const detail = failure === "failed" && error instanceof Error
                    ? error.message
                    : assistantFailureDetail(failure, assistantResponseTimeoutMs);
                setMessages((current) => [...current, {
                    id: nanoid(),
                    role: failure === "operator-cancelled" ? "notice" : "error",
                    content: detail,
                }]);
            }
        } finally {
            if (activeRequest.current === request) activeRequest.current = null;
            setIsLoading(false);
        }
    }

    function stopResponse() {
        activeRequest.current?.cancel("operator");
    }

    const statusLabel = status === "configured"
        ? `⊢ configured${model ? ` · ${model}` : ""}`
        : status === "checking" ? "○ checking configuration" : "○ unavailable";

    return (
        <div className="flex h-full flex-col overflow-hidden bg-background">
            <header className="flex min-h-16 items-center justify-between gap-4 border-b border-border px-5 py-3">
                <div className="flex min-w-0 items-center gap-3">
                    <div className="flex size-9 shrink-0 items-center justify-center border border-border">
                        <Command className="size-4" aria-hidden="true" />
                    </div>
                    <div className="min-w-0">
                        <h2 className="truncate text-sm font-semibold">Operator assistant</h2>
                        <p className="mt-1 truncate font-mono text-[11px] uppercase tracking-[0.08em] text-muted-foreground">{statusLabel}</p>
                    </div>
                </div>
                <span className="ck-claim ck-claim--muted">○ advisory</span>
            </header>

            <ScrollArea ref={scrollRef} className="flex-1 p-5">
                {messages.length === 0 ? (
                    <ConversationEmptyState
                        className="border border-dashed border-border"
                        title={status === "unavailable" ? "Assistant not configured" : "Advisory conversation"}
                        description={status === "unavailable"
                            ? statusDetail || "Set the gateway URL, API key, and model environment variables to enable this surface."
                            : "Ask about Kubernetes and operator concepts. The assistant has no cluster tools and cannot execute changes."}
                    />
                ) : (
                    <div className="flex flex-col gap-6" role="log" aria-live="polite">
                        {messages.map((message) => message.role === "error" || message.role === "notice" ? (
                            <div className="border border-dashed border-border p-4 text-sm text-muted-foreground" key={message.id}>
                                <p className="ck-label">{message.role === "notice" ? "Request stopped" : "Request failed"}</p>
                                <p className="mt-2 leading-6">{message.content}</p>
                            </div>
                        ) : (
                            <Message from={message.role} key={message.id}>
                                <p className="ck-label">{message.role === "user" ? "Operator" : "Assistant"}</p>
                                <MessageContent className={message.role === "user"
                                    ? "border border-primary bg-primary px-4 py-3 text-primary-foreground"
                                    : "border border-border bg-muted px-4 py-3"}
                                >
                                    <MessageResponse>{message.content}</MessageResponse>
                                </MessageContent>
                            </Message>
                        ))}
                        {isLoading && messages.at(-1)?.role !== "assistant" ? (
                            <p className="font-mono text-[11px] uppercase tracking-[0.08em] text-muted-foreground">○ Awaiting gateway response…</p>
                        ) : null}
                    </div>
                )}
            </ScrollArea>

            <div className="border-t border-border p-5">
                <form aria-busy={isLoading} className="border border-border p-3" onSubmit={handleSubmit}>
                    <label className="ck-label" htmlFor="operator-assistant-input">Message</label>
                    <textarea
                        className="mt-2 min-h-24 w-full resize-y bg-transparent px-1 py-2 text-sm leading-6 outline-none placeholder:text-muted-foreground"
                        disabled={status !== "configured" || isLoading}
                        id="operator-assistant-input"
                        maxLength={12_000}
                        onChange={(event) => setInput(event.target.value)}
                        onKeyDown={(event) => {
                            if (event.key === "Enter" && !event.shiftKey) {
                                event.preventDefault();
                                void handleSubmit();
                            }
                        }}
                        placeholder={status === "configured" ? "Ask an advisory question…" : "Assistant unavailable"}
                        rows={3}
                        value={input}
                    />
                    <div className="flex items-center justify-between gap-4 border-t border-border pt-3">
                        <p className="font-mono text-[11px] text-muted-foreground">No cluster execution</p>
                        {isLoading ? (
                            <Button onClick={stopResponse} type="button" variant="outline">Stop response</Button>
                        ) : (
                            <Button
                                aria-label="Send message"
                                className="min-h-11 min-w-11"
                                disabled={status !== "configured" || input.trim().length === 0}
                                size="icon"
                                type="submit"
                            >
                                <SendIcon className="size-4" aria-hidden="true" />
                            </Button>
                        )}
                    </div>
                </form>
            </div>
        </div>
    );
}
