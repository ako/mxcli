/**
 * ako/mxcli#574 — a URL-fed chart drew axes and a full legend with zero data
 * points, because the fetch Vega made carried the session cookie and nothing
 * else, and Mendix answers a session-authenticated OData read `401` without
 * `X-Csrf-Token`.
 *
 * Run with node's own test runner, no dependencies and no build:
 *
 *     node --test .claude/skills/packs/mendix-vega-charts/widget/test/csrf.test.ts
 *
 * or `make check-skill-pack-js`.
 */
import { test } from "node:test";
import assert from "node:assert/strict";

import { CSRF_HEADER, isSameOrigin, readCsrfToken, withCsrfHeader } from "../src/csrf.ts";

const APP = "http://localhost:8080/p/home";
const TOKEN = "6e8a0d3c-1f4b-4a21-9c77-0c2f5b1e4d90";

function headerOf(options: RequestInit): string | undefined {
    return (options.headers as Record<string, string> | undefined)?.[CSRF_HEADER];
}

// The reported symptom, at the layer the request is assembled: the fetch the
// widget makes for a relative spec URL must present the session's CSRF token.
// Without it Mendix answers 401 and Vega renders the empty chart.
test("a relative URL — the spec form the skill documents — carries the token", () => {
    const options = withCsrfHeader("/odata/chartapi/v1/Rows?$filter=Yr eq 2026", undefined, APP, TOKEN);
    assert.equal(headerOf(options), TOKEN, `${CSRF_HEADER} missing: Mendix answers this request 401`);
    assert.equal(options.credentials, "same-origin");
});

test("an absolute URL naming the app's own origin is the app, and carries it too", () => {
    const options = withCsrfHeader("http://localhost:8080/odata/chartapi/v1/Rows", undefined, APP, TOKEN);
    assert.equal(headerOf(options), TOKEN);
});

// The other half of the fix, and the reason it is not an unconditional header:
// the token authenticates this session against this app.
test("a third-party host is never handed the token", () => {
    const options = withCsrfHeader("https://data.example.org/rows.json", undefined, APP, TOKEN);
    assert.equal(headerOf(options), undefined);
    assert.equal(options.credentials, undefined);
});

// The case a `/^[a-z][a-z0-9+.-]*:\/\//i` test on the URI gets wrong: no scheme,
// so it reads as relative, while the request goes to another host.
test("a protocol-relative URL is a third-party host, not a relative path", () => {
    const options = withCsrfHeader("//data.example.org/rows.json", undefined, APP, TOKEN);
    assert.equal(headerOf(options), undefined);
});

test("an opaque or unparseable URI gets no token", () => {
    assert.equal(headerOf(withCsrfHeader("data:application/json,[]", undefined, APP, TOKEN)), undefined);
    assert.equal(headerOf(withCsrfHeader("http://[", undefined, APP, TOKEN)), undefined);
});

// Vega reuses one options object across the fetches of a spec. Writing into it
// would send the token to every later URL, third-party ones included.
test("the caller's options are not mutated", () => {
    const shared: RequestInit = { headers: { Accept: "application/json" } };
    const options = withCsrfHeader("/odata/chartapi/v1/Rows", shared, APP, TOKEN);
    assert.equal(headerOf(options), TOKEN);
    assert.equal(headerOf(shared), undefined, "the token leaked into the shared options object");
    assert.equal((options.headers as Record<string, string>).Accept, "application/json");
});

test("no token available — still same-origin credentials, no empty header", () => {
    const options = withCsrfHeader("/odata/chartapi/v1/Rows", undefined, APP, undefined);
    assert.equal(options.credentials, "same-origin");
    assert.equal(headerOf(options), undefined);
});

test("isSameOrigin resolves rather than string-matches", () => {
    assert.equal(isSameOrigin("rows.json", APP), true);
    assert.equal(isSameOrigin("../odata/v1/Rows", APP), true);
    assert.equal(isSameOrigin("HTTP://LOCALHOST:8080/odata/v1/Rows", APP), true);
    assert.equal(isSameOrigin("http://localhost:8081/odata/v1/Rows", APP), false, "a different port is a different origin");
    assert.equal(isSameOrigin("https://localhost:8080/odata/v1/Rows", APP), false, "a different scheme is a different origin");
});

// `mx` exists only in the Mendix client. The widget also runs in the Studio Pro
// preview and here, so reading the token must not throw.
test("readCsrfToken tolerates every shape of absent client", () => {
    const g = globalThis as { mx?: unknown };
    const saved = g.mx;
    try {
        delete g.mx;
        assert.equal(readCsrfToken(), undefined);
        g.mx = {};
        assert.equal(readCsrfToken(), undefined);
        g.mx = { session: {} };
        assert.equal(readCsrfToken(), undefined);
        g.mx = { session: { getConfig: () => "" } };
        assert.equal(readCsrfToken(), undefined, "an empty token is no token");
        g.mx = { session: { getConfig: (k: string) => (k === "csrftoken" ? TOKEN : undefined) } };
        assert.equal(readCsrfToken(), TOKEN);
    } finally {
        if (saved === undefined) {
            delete g.mx;
        } else {
            g.mx = saved;
        }
    }
});
