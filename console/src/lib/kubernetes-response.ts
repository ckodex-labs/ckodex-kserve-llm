export function kubernetesResponseBody<T>(response: T | { body: T }): T {
    if (typeof response === "object" && response !== null && "body" in response) {
        return response.body;
    }
    return response;
}
