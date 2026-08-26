import { createServer } from "node:http";

const port = Number(process.env.CKODEX_FAKE_AI_PORT || "3120");

function event(response, payload) {
    response.write(`data: ${JSON.stringify(payload)}\n\n`);
}

const server = createServer((request, response) => {
    if (request.method !== "POST" || request.url !== "/v1/responses") {
        response.writeHead(404).end();
        return;
    }

    request.resume();
    request.on("end", () => {
        response.writeHead(200, {
            "Cache-Control": "no-store",
            "Content-Type": "text/event-stream; charset=utf-8",
        });
        event(response, {
            type: "response.created",
            response: { id: "resp-local", created_at: 1_786_000_000, model: "operator-test", service_tier: null },
        });
        event(response, {
            type: "response.output_item.added",
            output_index: 0,
            item: { type: "message", id: "msg-local", phase: "final_answer" },
        });

        let index = 0;
        const interval = setInterval(() => {
            event(response, {
                type: "response.output_text.delta",
                item_id: "msg-local",
                delta: index++ === 0 ? "Local advisory stream active. " : "Observed chunk. ",
            });
        }, 250);

        response.on("close", () => clearInterval(interval));
    });
});

server.listen(port, "127.0.0.1", () => {
    console.log(`Fake AI gateway listening on http://127.0.0.1:${port}/v1`);
});
