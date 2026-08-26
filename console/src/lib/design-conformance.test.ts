import assert from "node:assert/strict";
import { readFileSync, readdirSync, statSync } from "node:fs";
import { join, relative } from "node:path";
import test from "node:test";

const consoleRoot = new URL("../../", import.meta.url);

function read(path: string) {
    return readFileSync(new URL(path, consoleRoot), "utf8");
}

function filesBelow(path: string, extensions: string[]) {
    const root = new URL(path, consoleRoot);
    const rootPath = root.pathname;
    const found: string[] = [];

    function walk(directory: string) {
        for (const entry of readdirSync(directory)) {
            const absolute = join(directory, entry);
            if (statSync(absolute).isDirectory()) walk(absolute);
            else if (extensions.some((extension) => entry.endsWith(extension))) found.push(absolute);
        }
    }

    walk(rootPath);
    return found.map((absolute) => ({
        path: relative(rootPath, absolute),
        content: readFileSync(absolute, "utf8"),
    }));
}

test("binds all active product components to the DS-3 semantic palette", () => {
    const files = [
        ...filesBelow("src/components/features/", [".tsx", ".ts", ".css"]),
        ...filesBelow("src/components/layout/", [".tsx", ".ts", ".css"]),
        ...filesBelow("src/components/ai-elements/", [".tsx", ".ts", ".css"]),
    ];
    const rawColour = /#[\da-f]{3,8}\b|\brgb\(|\bhsl\(/i;
    const forbiddenDecoration = /\b(?:bg|from|via|to)-gradient\b|\bshadow-(?!none\b)/;

    for (const file of files) {
        assert.doesNotMatch(file.content, rawColour, `${file.path} contains a raw colour`);
        assert.doesNotMatch(file.content, forbiddenDecoration, `${file.path} contains non-DS-3 decoration`);
    }
});

test("keeps DS-3 themes, motion suppression, and forced colours explicit", () => {
    const css = read("src/app/globals.css");
    const layout = read("src/app/[locale]/layout.tsx");
    const themeControl = read("src/components/layout/ThemeControl.tsx");

    assert.match(layout, /data-theme="ledger"/);
    assert.match(themeControl, /useState<Theme>\("ledger"\)/);
    assert.match(css, /\[data-theme="vault"\]/);
    assert.match(css, /\[data-theme="hc"\]/);
    assert.match(css, /@media \(forced-colors: active\)/);
    assert.match(css, /@media \(prefers-reduced-motion: reduce\)/);
    assert.match(css, /:focus-visible/);
});

test("routes share one operator shell and the skip link precedes application providers", () => {
    const pages = filesBelow("src/app/[locale]/(dashboard)/", ["page.tsx"]);
    for (const page of pages) {
        assert.match(page.content, /<OperatorPage\b/, `${page.path} bypasses OperatorPage`);
    }

    const layout = read("src/app/[locale]/layout.tsx");
    const skip = layout.indexOf('className="ck-skip"');
    const provider = layout.indexOf("<NextIntlClientProvider");
    assert.ok(skip >= 0 && provider >= 0 && skip < provider, "skip link must precede the application shell");
});

test("active product copy excludes forbidden glyphs and first-person address", () => {
    const files = [
        ...filesBelow("src/components/features/", [".tsx"]),
        ...filesBelow("src/components/layout/", [".tsx"]),
    ];
    const forbiddenGlyph = /[✓✗🚀⚠️✅❌🔒]/u;
    const firstPersonAddress = /\b(?:you|your|we|our)\b/i;

    for (const file of files) {
        assert.doesNotMatch(file.content, forbiddenGlyph, `${file.path} contains a forbidden glyph`);
        assert.doesNotMatch(file.content, firstPersonAddress, `${file.path} contains first-person product copy`);
    }
});

test("shared button and navigation adapters preserve minimum targets and visible disabled states", () => {
    const button = read("src/components/ui/button.tsx");
    const sidebar = [
        read("src/components/ui/sidebar-layout.tsx"),
        read("src/components/ui/sidebar-menu.tsx"),
    ].join("\n");

    assert.match(button, /min-h-11 min-w-11/);
    assert.match(button, /disabled:opacity-100/);
    assert.match(sidebar, /group-data-\[collapsible=icon\]:size-11!/);
    assert.match(sidebar, /disabled:opacity-100/);
});

test("shared UI adapters cannot reintroduce non-DS-3 substrate styling", () => {
    const files = filesBelow("src/components/ui/", [".tsx", ".ts"]);
    const forbiddenDropShadow = /\bshadow-(?:sm|md|lg|xl|2xl)\b/;
    const forbiddenLoopingMotion = /\banimate-(?:spin|pulse)\b/;
    const destructiveValidation = /aria-invalid:[^\s"]*destructive/;
    const opacityOnlyDisabled = /(?:disabled|data-disabled|aria-disabled):opacity-(?!100\b)/;

    for (const file of files) {
        assert.doesNotMatch(file.content, forbiddenDropShadow, `${file.path} contains a drop shadow`);
        assert.doesNotMatch(file.content, forbiddenLoopingMotion, `${file.path} contains looping status motion`);
        assert.doesNotMatch(file.content, destructiveValidation, `${file.path} uses emergency colour for validation`);
        assert.doesNotMatch(file.content, opacityOnlyDisabled, `${file.path} fades a disabled control`);
        assert.doesNotMatch(file.content, /rounded-full/, `${file.path} contains pill or circular geometry`);
    }
});

test("the DS-3 adapter enforces square geometry and ink validation globally", () => {
    const css = read("src/app/globals.css");

    assert.match(css, /border-radius: 0 !important/);
    assert.match(css, /:disabled,[\s\S]*\[aria-disabled="true"\],[\s\S]*\[data-disabled\][\s\S]*opacity: 1 !important/);
    assert.match(css, /\[aria-invalid="true"\][\s\S]*border-color: var\(--ck-ink\) !important/);
    assert.match(css, /box-shadow: inset 0 -2px 0 var\(--ck-ink\) !important/);
});

test("evidence margin remains operable and container-aware at every width", () => {
    const shell = read("src/components/layout/OperatorShell.tsx");
    const css = read("src/app/globals.css");

    assert.match(shell, /aria-label="Collapse evidence margin"/);
    assert.match(shell, /aria-label="Expand evidence margin"/);
    assert.match(shell, /Copy source reference/);
    assert.match(shell, /data-evidence-collapsed/);
    assert.match(css, /@container operator-shell \(max-width: 900px\)/);
    assert.match(css, /\.ck-evidence-margin__rail/);
    assert.match(css, /data-evidence-collapsed="true"[\s\S]*\.ck-evidence-margin__body \{ display: block; \}/);
});

test("global operator navigation is keyboard-complete and shares one route registry", () => {
    const palette = read("src/components/layout/OperatorCommandPalette.tsx");
    const sidebar = read("src/components/layout/AppSidebar.tsx");
    const navigation = read("src/components/layout/operator-navigation.ts");
    const css = read("src/app/globals.css");

    assert.match(palette, /aria-keyshortcuts="Meta\+K Control\+K"/);
    assert.match(palette, /event\.metaKey && !event\.ctrlKey/);
    assert.match(palette, /getOperatorNavigation\(profile\)/);
    assert.match(sidebar, /getOperatorNavigation\(profile\)/);
    assert.match(sidebar, /mobileMode="inline"/);
    assert.match(sidebar, /aria-current=\{current \? "page"/);
    assert.match(sidebar, /scrollIntoView\(\{ block: "nearest", inline: "center" \}\)/);
    assert.match(css, /@media \(max-width: 900px\)[\s\S]*\.ck-primary-nav/);
    assert.match(css, /\.ck-primary-nav__link\[aria-current="page"\]/);
    assert.match(navigation, /url: "\/accelerators"/);
    assert.match(palette, /Refresh sources/);
});

test("all refresh entry points share the completed server-transition lifecycle", () => {
    const provider = read("src/components/layout/OperatorRefreshProvider.tsx");
    const control = read("src/components/layout/RefreshControl.tsx");
    const poller = read("src/components/features/LivePoller.tsx");
    const palette = read("src/components/layout/OperatorCommandPalette.tsx");
    const layout = read("src/app/[locale]/layout.tsx");

    assert.match(layout, /<OperatorRefreshProvider>/);
    assert.match(provider, /useTransition\(\)/);
    assert.match(provider, /setLastCompletedAt\(new Date\(\)\)/);
    assert.match(control, /useOperatorRefresh\(\)/);
    assert.doesNotMatch(control, /setTimeout/);
    assert.match(poller, /lastCompletedAt/);
    assert.match(poller, /aria-live="polite"/);
    assert.match(palette, /useOperatorRefresh\(\)/);
    assert.match(palette, /disabled=\{isRefreshing\}/);
});

test("investigation surfaces share one filter lifecycle and native-select adapter", () => {
    const filterBar = read("src/components/layout/OperatorFilterBar.tsx");
    const nativeSelect = read("src/components/ui/native-select.tsx");
    const searchState = read("src/hooks/use-investigation-search-state.ts");
    const surfaces = [
        read("src/components/features/fleet/index.tsx"),
        read("src/components/features/audit/index.tsx"),
        read("src/components/features/telemetry/TelemetryView.tsx"),
    ];

    assert.match(filterBar, /aria-live="polite"/);
    assert.match(filterBar, /Reset filters/);
    assert.match(filterBar, /disabled=\{!hasActiveFilters\}/);
    assert.match(nativeSelect, /data-slot="native-select"/);
    assert.match(nativeSelect, /disabled:opacity-100/);
    assert.match(nativeSelect, /aria-invalid:border-foreground/);
    assert.match(searchState, /buildInvestigationPath/);
    assert.match(searchState, /window\.history\.replaceState/);

    for (const surface of surfaces) {
        assert.match(surface, /<OperatorFilterBar/);
        assert.match(surface, /<Input/);
        assert.match(surface, /<NativeSelect/);
        assert.match(surface, /onReset=/);
        assert.match(surface, /useInvestigationSearchState/);
    }
});

test("wide operator tables expose named, keyboard-scrollable sticky viewports", () => {
    const css = read("src/app/globals.css");
    const tableSurfaces = [
        read("src/components/features/audit/index.tsx"),
        read("src/components/features/identity/index.tsx"),
        read("src/components/features/infrastructure/NodeManagerBlock.tsx"),
        read("src/components/features/infrastructure/AcceleratorFleetBlock.tsx"),
        read("src/components/features/fleet/WorkloadDetail.tsx"),
    ];

    assert.match(css, /\.ck-table-scroll[\s\S]*overflow-x: auto/);
    assert.match(css, /\.ck-table-scroll thead[\s\S]*position: sticky/);
    assert.match(css, /@container operator-main \(min-width: 760px\)[\s\S]*\.ck-fleet-filter-fields/);
    assert.match(css, /@container operator-main \(min-width: 760px\)[\s\S]*\.ck-telemetry-filter-fields/);

    for (const surface of tableSurfaces) {
        assert.match(surface, /aria-label="[^"]+ table"/);
        assert.match(surface, /className="ck-table-scroll/);
        assert.match(surface, /role="region"/);
        assert.match(surface, /tabIndex=\{0\}/);
    }

    assert.match(tableSurfaces[1], /aria-label="Capability matrix table"/);
});

test("telemetry investigation controls govern metrics and alerts from the page level", () => {
    const telemetry = read("src/components/features/telemetry/TelemetryView.tsx");
    const telemetryPage = read("src/app/[locale]/(dashboard)/telemetry/page.tsx");
    const filter = telemetry.indexOf("<OperatorFilterBar");
    const metrics = telemetry.indexOf('<section aria-labelledby="metric-series-title">');
    const alerts = telemetry.indexOf('<section className="mt-8" aria-labelledby="active-alerts-title">');

    assert.ok(filter >= 0 && filter < metrics && metrics < alerts);
    assert.match(telemetry, /fieldsClassName="ck-telemetry-filter-fields"/);
    assert.match(telemetry, />Alert state</);
    assert.match(telemetryPage, /safeTelemetryLink\(process\.env\.CKODEX_GRAFANA_DASHBOARD_URL\)/);
    assert.match(telemetryPage, /rel="noreferrer"/);
});

test("route segments share evidence-aware loading and recovery boundaries", () => {
    const routeState = read("src/components/layout/OperatorRouteState.tsx");
    const loading = read("src/app/[locale]/(dashboard)/loading.tsx");
    const error = read("src/app/[locale]/(dashboard)/error.tsx");
    const notFound = read("src/app/[locale]/not-found.tsx");
    const globalNotFound = read("src/app/global-not-found.tsx");
    const globalError = read("src/app/global-error.tsx");
    const nextConfig = read("next.config.ts");
    const css = read("src/app/globals.css");

    assert.match(loading, /<OperatorRouteState/);
    assert.match(error, /unstable_retry/);
    assert.match(error, /<OperatorRouteState/);
    assert.match(notFound, /<OperatorRouteState/);
    assert.match(routeState, /<aside[^>]+aria-labelledby="route-evidence-title"/);
    assert.match(routeState, /evidence unsealed/);
    assert.match(routeState, /mode changes deployment, not governance semantics/);
    assert.match(globalError, /<html[^>]+data-theme="ledger"/);
    assert.match(globalError, /unstable_retry/);
    assert.match(globalNotFound, /<html[^>]+data-theme="ledger"/);
    assert.match(globalNotFound, /No source or readiness observation is asserted/);
    assert.match(nextConfig, /globalNotFound: true/);
    assert.match(css, /\.ck-route-shell/);
    assert.match(css, /@media \(max-width: 900px\)[\s\S]*\.ck-route-shell/);
});

test("operator assistant requests are cancellable and bounded on both sides", () => {
    const panel = read("src/components/features/AISidePanel.tsx");
    const chat = read("src/components/features/ChatInterface.tsx");
    const helper = read("src/lib/assistant-request.ts");
    const route = read("src/app/api/chat/route.ts");
    const gatewayUrl = read("src/lib/ai-gateway-url.ts");

    assert.match(panel, /<ChatInterface active=\{open\}/);
    assert.match(helper, /AbortSignal\.timeout\(timeoutMs\)/);
    assert.match(helper, /AbortSignal\.any/);
    assert.match(chat, /activeRequest\.current\?\.cancel\("lifecycle"\)/);
    assert.match(chat, /Stop response/);
    assert.match(chat, /signal: request\.signal/);
    assert.match(route, /abortSignal: req\.signal/);
    assert.match(route, /timeout: \{ totalMs: timeoutMs, chunkMs:/);
    assert.match(route, /"Cache-Control": "no-store"/);
    assert.match(route, /CKODEX_AI_GATEWAY_ALLOW_INSECURE/);
    assert.match(route, /resolveAiGatewayBaseUrl/);
    assert.match(gatewayUrl, /url\.username \|\| url\.password \|\| url\.search \|\| url\.hash/);
});
