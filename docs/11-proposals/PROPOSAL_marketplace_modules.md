---
title: mxcli marketplace — Download & Manage Marketplace Modules
status: partial
date: 2026-06-05
---

# Proposal: `mxcli marketplace` — Download & Manage Marketplace Modules

**Status:** Shipping — discovery + download + type-aware install; module update postponed
**Date:** 2026-03-23 (initial), revised 2026-04-16 (spike results), 2026-06-05 (download unblocked)
**Author:** Generated with Claude Code

## Status update (2026-06-05) — download UNBLOCKED

Unblocking path #1 below has happened: the content API **now returns a
`downloadUrl`** on each version (verified live 2026-06-05; see the
`reference_marketplace_download_api` memory). `GET .../v1/versions/{id}/download`
with `MxToken` auth 303-redirects to the public CDN `.mpk`. Shipped:

- `marketplace download <id> [--version X] [-o file]` — fetch the `.mpk` to disk.
- `marketplace install <id> -p app.mpr` — type-aware: **widget** → `widgets/`,
  **module (new)** → `mx module-import`, other types → download + instruct.

**Module *updates* remain postponed** (detect + report only, no mutation):
re-importing an existing module would discard local edits and, for modules with
persistent entities, change entity `$ID`s — losing runtime data. Studio Pro's
Marketplace "Update" does an ID-preserving merge the `mx` CLI does not expose.
A future ID-preserving merge is the remaining work.

## Status update (2026-04-16) — historical (now resolved)

After four rounds of spiking (`scripts/auth-discovery-spike.sh`), the
**install** path is blocked by a gap in Mendix's API: there is no way to
obtain the `.mpk` download URL with a Personal Access Token.

- The API at `marketplace-api.mendix.com` does not expose `downloadUrl` or
  any equivalent field on content or version objects.
- The CDN at `files.appstore.mendix.com` is public for GET, but its path
  format (`/{N}/{M}/{version}/{filename}.mpk`) contains three opaque
  per-module values (N, M, and the filename convention) that the API does
  not return and we cannot derive.
- The marketplace website at `marketplace.mendix.com/link/component/{id}`
  contains the download link in its HTML, but requires AAD SSO — PATs
  return a 404/login page.
- `mx` (mxbuild) has no marketplace subcommand; only `module-import` for
  an already-downloaded `.mpk`.

This proposal is therefore scoped in two phases:

- **Phase A — ship now**: read-only discovery commands (`search`, `info`,
  `versions`). These work with the existing PAT auth layer and the
  confirmed API endpoints. They deliver concrete user value (CLI
  marketplace browsing, version-compat filtering) without the install gap.
- **Phase B — parked**: `install` and `update` commands. Unblocked by
  either Mendix adding `downloadUrl` to the API, or a future AAD device-
  code auth flow that could access the web app HTML (see
  `PROPOSAL_platform_auth.md` Phase 6).

## Problem

Mendix projects depend on marketplace modules (e.g., ExternalDatabaseConnector, BusinessEvents) that are not bundled with the Mendix runtime. Currently, these modules must be manually downloaded from the Mendix Marketplace website and imported via Studio Pro or `mx module-import`.

This creates friction in two areas:

1. **CI/CD pipelines** — Integration tests that exercise database connector or business event features require marketplace modules. We currently bundle `.mpk` files in `mx-modules/`, which is not scalable and requires manual updates.
2. **Project setup** — When `mxcli init` creates a project or when MDL scripts reference marketplace modules, there's no way to auto-install dependencies.

## API Discovery

Validated against a real PAT on 2026-04-14 (see `scripts/auth-discovery-spike.sh`).

**Base URL:** `https://marketplace-api.mendix.com`

(An earlier draft of this proposal pointed at `appstore.home.mendix.com/rest/packagesapi/v2/`.
That host is a different service and does not accept PAT auth at all. The correct marketplace
host is `marketplace-api.mendix.com`.)

**Auth:** `Authorization: MxToken <pat>`. PATs are created at
<https://user-settings.mendix.com/> (Developer Settings → Personal Access Tokens).
Invalid/missing PAT returns 401 or 403 with a JSON error body; malformed tokens
may be rejected at the gateway with 400.

### Validated Endpoints

| Endpoint | Returns | Purpose |
|----------|---------|---------|
| `get /v1/content` | `{"items": [content, ...]}` | List marketplace content |
| `get /v1/content?search=<query>` | same list shape | Search (query accepted; filter behavior TBD) |
| `get /v1/content/{id}` | single content object | Module/widget detail |
| `get /v1/content/{id}/versions` | `{"items": [version, ...]}` | Available versions with compatibility metadata |

### Response Shapes

**Content object** (from `/v1/content` and `/v1/content/{id}`):

```jsonc
{
  "contentId": 2888,
  "publisher": "Mendix",
  "type": "module",             // or "widget", "Theme", etc.
  "categories": [{"name": "data"}],
  "supportCategory": "Platform", // or "Community", "deprecated", ...
  "licenseUrl": "http://www.apache.org/licenses/LICENSE-2.0.html",
  "isPrivate": false,
  // ...more fields including latest version info (not yet fully mapped)
}
```

**Version object** (from `/v1/content/{id}/versions`):

```jsonc
{
  "name": "database connector",
  "versionId": "f7c2bddf-05a3-4db0-8185-e7adf6c6d4af",  // uuid
  "versionNumber": "7.0.2",
  "minSupportedMendixVersion": "10.24.11",               // enables version-compat filtering
  "publicationDate": "2025-12-12T08:08:53.880Z"
  // release notes, download url(s) TBD — need to inspect full response
}
```

### Download URL — blocked

The `.mpk` download URL follows the pattern:

```
https://files.appstore.mendix.com/<company-id>/<component-id>/<version>/<filename>.mpk
```

None of the path components are returned by the marketplace API:

| Path segment | Example | In API response? |
|---|---|---|
| `company-id` | `5` (Mendix), `50537` (third party) | No |
| `component-id` | `170`, `219862` — differs from the API's `contentId` | No |
| `version` | `11.5.0` | Yes (`versionNumber`) — but useless without the other segments |
| `filename` | Varies per module (`CommunityCommons_11.5.0.mpk`, `ExternalDatabaseConnector-v6.2.4.mpk`, `JamAuditLog.mpk`) | No |

The CDN itself is **public** (no auth needed for GET), but the URL cannot be
constructed from API data. The marketplace website at `marketplace.mendix.com`
contains the download links but requires AAD SSO — PATs do not work against
the web app.

**Unblocking paths:**
1. Mendix adds a `downloadUrl` field to the content or versions API response
2. `mxcli` gains AAD device-code auth (see `PROPOSAL_platform_auth.md` Phase 6)
   to access the web app and scrape the download link
3. Mendix publishes a CLI-accessible download endpoint (e.g., `post /v1/content/{id}/versions/{versionId}/download` returning a signed URL)

### Open Endpoint Questions

- **Search semantics**: `?search=database` accepted without error but truncated output made
  it unclear whether the result set was actually filtered vs. returned unchanged.

### Known Component IDs

| Module | Content ID | Used By |
|--------|-----------|---------|
| External Database Connector | 219862 | 05-database-connection-examples.mdl |
| Database Connector (separate JDBC module — not the one used by 05) | 2888 | — |
| Business Events | (TBD) | 13-business-events-examples.mdl |

> Note: `219862` carries the `6.2.x` line (e.g. `v6.2.3`, the bundled fixture version);
> `2888` is a different module ("Database Connector") with its own `7.0.x`/`6.0.x` line. An
> earlier draft conflated the two and labelled External Database Connector as `2888`.

## Proposed Commands

### Authentication

```bash
# Interactive login (prompts for email + api key or PAT)
mxcli auth login

# non-interactive login (for CI)
mxcli auth login --token <PAT>
mxcli auth login --username user@company.com --api-key <KEY>

# check auth status
mxcli auth status

# clear stored credentials
mxcli auth logout
```

### Marketplace Operations

```bash
# search marketplace
mxcli marketplace search "database connector"

# show module details
mxcli marketplace info 219862

# Install module into project (219862 = External Database Connector; 6.2.3 is one of its versions)
mxcli marketplace install 219862 -p app.mpr
mxcli marketplace install 219862 --version 6.2.3 -p app.mpr

# list installed marketplace modules
mxcli marketplace list -p app.mpr

# update all marketplace modules to latest compatible version
mxcli marketplace update -p app.mpr

# update specific module
mxcli marketplace update 2888 -p app.mpr
```

## Architecture

### File Structure

```
cmd/mxcli/
├── auth.go                 # login/logout/status commands
├── cmd_marketplace.go      # search/install/update/list commands

internal/
├── auth/
│   ├── store.go            # Credential storage (~/.mxcli/auth.json)
│   └── client.go           # Authenticated HTTP client factory
│
├── marketplace/
│   ├── api.go              # rest api client for appstore.home.mendix.com
│   ├── search.go           # search and list components
│   ├── download.go         # Download .mpk files (with progress bar)
│   └── install.go          # import via mx module-import + version tracking
```

### Credential Storage

Credentials stored at `~/.mxcli/auth.json` with `0600` permissions:

```json
{
  "email": "user@company.com",
  "api_key": "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx",
  "created_at": "2026-03-23T12:00:00Z"
}
```

Environment variables take precedence for CI:

```bash
export MENDIX_USERNAME=user@company.com
export MENDIX_API_KEY=xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
```

### Install Flow

```
mxcli marketplace install 2888 -p app.mpr
  │
  ├─ 1. Load credentials (env vars → ~/.mxcli/auth.json)
  ├─ 2. get /packages/2888 → module name, available versions
  ├─ 3. filter versions compatible with project's Mendix version
  ├─ 4. get /packages/2888/versions/{ver}/download → download .mpk to temp
  ├─ 5. mx module-import /tmp/module.mpk app.mpr
  └─ 6. log: "Installed ExternalDatabaseConnector v6.2.3"
```

### Integration with Existing Code

The download infrastructure in `cmd/mxcli/docker/download.go` provides a reusable pattern:
- HTTP download with progress bar
- Temporary file handling
- Archive extraction

The `mx module-import MPK_PATH MPR_PATH` command (already available in mxbuild) handles the actual module import.

Module metadata fields already exist in `model.Module`:
- `AppStoreGuid` — Marketplace component identifier
- `AppStoreVersion` — Installed version
- `FromAppStore` — Whether module came from marketplace

## Integration with Tests

### Current approach (Phase 1)

Marketplace `.mpk` files bundled in `mx-modules/`:

```go
// roundtrip_doctype_test.go
var scriptModuleDeps = map[string][]string{
    "05-database-connection-examples.mdl": {"ExternalDatabaseConnector-v6.2.3.mpk"},
    "13-business-events-examples.mdl":     {"BusinessEvents_3.12.0.mpk"},
}
```

### Future approach (Phase 3+)

Tests use `mxcli marketplace install` with fallback to local `.mpk`:

```go
func importMarketplaceModule(t *testing.T, componentID string, mprPath string) {
    // Try local mpk cache first (fast, no auth needed)
    localMPK := findLocalMPK(componentID)
    if localMPK != "" {
        runMxModuleImport(localMPK, mprPath)
        return
    }

    // Fall back to marketplace download (needs auth)
    if !hasMarketplaceAuth() {
        t.Skipf("marketplace module %s not available (no auth)", componentID)
        return
    }
    runMxcliMarketplaceInstall(componentID, mprPath)
}
```

## Implementation Phases

### Phase 1: Bundle .mpk files (current)

- `.mpk` files in `mx-modules/` directory
- Test imports via `mx module-import` before script execution
- **Status: Done**

### Phase 2: Authentication

See dedicated proposal: [`PROPOSAL_platform_auth.md`](PROPOSAL_platform_auth.md). The auth layer is shared with Deploy API and future platform-API consumers.

- `mxcli auth login/logout/status` commands
- Credential storage at `~/.mxcli/auth.json` (plus OS keychain)
- Environment variable support for CI
- Authenticated HTTP client factory with per-host scheme routing (PAT for Content API / marketplace; API key for Deploy API)

### Phase 3: Install & Download

- `mxcli marketplace install <id> -p app.mpr`
- Validate API endpoints with real credentials
- Download `.mpk` with progress bar
- Import via `mx module-import`
- Version compatibility filtering by project Mendix version

### Phase 4: Search & Management

- `mxcli marketplace search <query>`
- `mxcli marketplace list -p app.mpr`
- `mxcli marketplace update -p app.mpr`
- `mxcli marketplace info <id>`

### Phase 5: MDL Integration

- `INSTALL module <id> [version <version>];` MDL statement
- Auto-dependency resolution in `mxcli exec` when module references are missing

## Open Questions

1. **Which auth scheme does the marketplace API use?** Need to test with real credentials. Mendix platform APIs use either `Mendix-username + Mendix-ApiKey` headers or `MxToken` PAT tokens.

2. **Version compatibility filtering** — The API likely returns Mendix version compatibility metadata per module version. Need to confirm the response format.

3. **Protected modules** — Some marketplace modules are add-on/solution modules that `mx module-import` rejects (exit code X11). Need to handle gracefully.

4. **Rate limiting** — Unknown; need to test and add retry logic if needed.

5. **Component ID discovery** — Users may not know the numeric component ID. Search by module name (`mxcli marketplace search "database connector"`) addresses this, but we may also want to support names directly: `mxcli marketplace install DatabaseConnector`.

6. **Cache strategy** — Should downloaded `.mpk` files be cached in `~/.mxcli/marketplace/{id}/{version}/module.mpk` to avoid re-downloading?
