/**
 * Authenticating a Vega fetch against the app's own endpoints.
 *
 * A URL-fed spec — `{"data": {"url": "/odata/…"}}` — is fetched by Vega's own
 * loader, not by the Mendix client, so it carries whatever `fetch` carries by
 * default. Against a *session-authenticated* Mendix endpoint that is not
 * enough: Mendix refuses a session-authenticated request, **including a read**,
 * unless it also presents the session's CSRF token. Measured on 11.14.0, two
 * requests differing in one header:
 *
 *     session cookie only                  401
 *     session cookie + X-Csrf-Token        200
 *
 * The failure is silent in the worst way — Vega treats the 401 body as an empty
 * dataset, so the chart draws its axes and a full legend with no marks and no
 * error. See `references/failure-modes.md`.
 *
 * The token is not a cookie and cannot ride along by itself; the client keeps it
 * at `mx.session.getConfig("csrftoken")`.
 *
 * **Same-origin only.** The token authenticates this session against this app,
 * so attaching it to a request bound for another host hands that host a working
 * credential. A spec's `url` is author-controlled, but an author reaching a
 * public data set over `https://` must not thereby leak the session — so the
 * header is added only where the request is going back to the app itself.
 */

/** The header Mendix reads the session's CSRF token from. */
export const CSRF_HEADER = "X-Csrf-Token";

/**
 * Does `uri` resolve to the same origin as `base`?
 *
 * Resolution, not string matching, because the cases that matter do not look
 * alike. A protocol-relative `//elsewhere.example/rows.json` carries no scheme
 * and so passes any "does it start with https://" test, while going to another
 * host — which is exactly the request that must not carry the token. Conversely
 * an absolute URL naming the app's own origin is the app, and refusing it would
 * break a spec that spells its endpoint out in full.
 *
 * Anything that will not parse, and anything opaque (`data:`, `blob:`), is not
 * same-origin: no header, which is the safe answer for both.
 */
export function isSameOrigin(uri: string, base: string): boolean {
    try {
        return new URL(uri, base).origin === new URL(base).origin;
    } catch {
        return false;
    }
}

/**
 * Read the session's CSRF token from the Mendix client.
 *
 * Defensive about every step: the widget also runs in the Studio Pro preview
 * and in a unit test, where `mx` does not exist, and a chart that throws on a
 * missing global is worse than one that fetches without a token.
 */
export function readCsrfToken(): string | undefined {
    const client = (globalThis as { mx?: { session?: { getConfig?: (key: string) => unknown } } }).mx;
    const token = client?.session?.getConfig?.("csrftoken");
    return typeof token === "string" && token !== "" ? token : undefined;
}

/**
 * The request options for one Vega fetch: the caller's, plus credentials and
 * the CSRF token where the request is going back to this app.
 *
 * Pure, and it never mutates what it is handed — Vega reuses its options object
 * across the fetches of one spec, so writing a header into it would send this
 * app's token to every later URL, including the third-party ones this function
 * exists to protect.
 */
export function withCsrfHeader(
    uri: string,
    options: RequestInit | undefined,
    base: string,
    token: string | undefined
): RequestInit {
    if (!isSameOrigin(uri, base)) {
        return { ...options };
    }
    const next: RequestInit = { ...options, credentials: "same-origin" };
    if (token) {
        next.headers = { ...(options?.headers as Record<string, string> | undefined), [CSRF_HEADER]: token };
    }
    return next;
}
