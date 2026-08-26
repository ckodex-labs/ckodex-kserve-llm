export const assistantConfigurationTimeoutMs = 5_000;
export const assistantResponseTimeoutMs = 35_000;

export type AssistantCancellationReason = "operator" | "lifecycle";
export type AssistantRequestFailure = "operator-cancelled" | "lifecycle-cancelled" | "deadline" | "failed";

export interface AssistantRequest {
    signal: AbortSignal;
    cancel: (reason: AssistantCancellationReason) => void;
    classifyFailure: (error: unknown) => AssistantRequestFailure;
}

export function createAssistantRequest(timeoutMs: number): AssistantRequest {
    if (!Number.isFinite(timeoutMs) || timeoutMs <= 0) {
        throw new RangeError("Assistant request timeout must be a positive finite number.");
    }

    const controller = new AbortController();
    const deadline = AbortSignal.timeout(timeoutMs);
    const signal = AbortSignal.any([controller.signal, deadline]);
    let cancellationReason: AssistantCancellationReason | null = null;

    return {
        signal,
        cancel(reason) {
            if (signal.aborted) return;
            cancellationReason = reason;
            controller.abort(reason);
        },
        classifyFailure(error) {
            if (cancellationReason === "operator") return "operator-cancelled";
            if (cancellationReason === "lifecycle") return "lifecycle-cancelled";
            if (deadline.aborted) return "deadline";
            if (error instanceof DOMException && error.name === "TimeoutError") return "deadline";
            return "failed";
        },
    };
}

export function assistantFailureDetail(failure: AssistantRequestFailure, timeoutMs: number): string {
    if (failure === "operator-cancelled") return "Response stopped by operator.";
    if (failure === "lifecycle-cancelled") return "Request closed with the assistant surface.";
    if (failure === "deadline") {
        const seconds = Number.isInteger(timeoutMs / 1_000) ? `${timeoutMs / 1_000} s` : `${timeoutMs} ms`;
        return `The assistant request reached the ${seconds} console deadline.`;
    }
    return "The assistant gateway request failed.";
}
