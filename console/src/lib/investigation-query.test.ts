import assert from "node:assert/strict";
import test from "node:test";

import {
    allowedInvestigationFilter,
    boundedInvestigationQuery,
    buildInvestigationPath,
    investigationQueryLimit,
} from "./investigation-query.ts";

test("bounds investigation queries and rejects array-shaped search parameters", () => {
    assert.equal(boundedInvestigationQuery("x".repeat(300)).length, investigationQueryLimit);
    assert.equal(boundedInvestigationQuery(["one", "two"]), "");
    assert.equal(boundedInvestigationQuery(undefined), "");
});

test("accepts only declared investigation filter values", () => {
    const allowed = ["all", "ready", "attention"] as const;
    assert.equal(allowedInvestigationFilter("ready", allowed, "all"), "ready");
    assert.equal(allowedInvestigationFilter("unknown", allowed, "all"), "all");
    assert.equal(allowedInvestigationFilter(["ready"], allowed, "all"), "all");
});

test("builds a bounded, shareable investigation path without retaining defaults", () => {
    assert.equal(
        buildInvestigationPath("https://console.example/en/fleet?unrelated=kept#records", {
            filterKey: "readiness",
            filterValue: "attention",
            query: "  model-a  ",
        }),
        "/en/fleet?unrelated=kept&q=model-a&readiness=attention#records",
    );
    assert.equal(
        buildInvestigationPath("https://console.example/en/fleet?q=old&readiness=ready", {
            filterKey: "readiness",
            filterValue: "all",
            query: "",
        }),
        "/en/fleet",
    );
});
