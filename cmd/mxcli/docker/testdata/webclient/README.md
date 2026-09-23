# Web client entry points, as mxbuild writes them

Both files are `deployment/web/index.html` from a real `mxbuild --target=deploy`
run of the same blank Mendix 11.12.2 app, differing only in
**Settings > Web UI > OptimizedClient**:

| OptimizedClient | `web/` holds | `web/index.html` loads | sibling dir |
|---|---|---|---|
| `Yes` (default) | the React client | `dist/index.js` | `dojo-web/` |
| `No` | the classic Dojo client | `mxclientsystem/mxui/mxui.js` | `react-web/` |

mxbuild swaps which client lands in `web/` and parks the other one beside it, so
the entry point names the client that is actually served. That is what
`planWebClient` reads, rather than the model's setting: a stale deployment and a
just-changed setting disagree, and it is the deployment that gets served.

A classic deployment has **no** `web/rollup.config.mjs` and **no** `web/dist/` —
there is no bundling step for the Dojo client — which is why the gate that
assumed one of those two must exist rejected the app outright (issue #1123).
