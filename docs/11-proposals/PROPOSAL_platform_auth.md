---
title: mxcli auth — Mendix Platform Authentication
status: draft
date: 2026-04-14
---

# Proposal: `mxcli auth` — Mendix Platform Authentication

**Status:** Draft
**Date:** 2026-04-14
**Author:** Generated with Claude Code

## Problem

A growing set of `mxcli` features need to talk to Mendix platform APIs on behalf of the user:

- **Marketplace** — downloading `.mpk` files (see `PROPOSAL_marketplace_modules.md`)
- **Deploy API** — building, packaging, and deploying apps to Mendix Cloud
- **Team Server, Content API, Platform APIs** — future features (project metadata, users, environments)

Each of these APIs uses a different authentication header scheme, and today `mxcli` has no concept of stored credentials. Hard-coding auth into each feature would produce duplicated credential-storage code, inconsistent UX, and no single place for the user to run `mxcli auth login`.

This proposal specifies a shared `internal/auth` package and `mxcli auth` command set that every platform-API consumer can use.

## Spike Results (2026-04-14)

Validated against real credentials. See `scripts/auth-discovery-spike.sh`.

- **Marketplace lives at `marketplace-api.mendix.com`, not `appstore.home.mendix.com`.**
  The older host is a different service that does not accept PAT auth at all.
- **PAT works as documented** — `Authorization: MxToken <pat>` (capital M, capital T)
  against `marketplace-api.mendix.com/v1/content` returns 200 with a well-formed JSON
  list. Catalog host also accepts the same header; paths TBD.
- **401 and 403** are both "credential rejected" — our `authTransport` wraps either
  as `ErrUnauthenticated`. A malformed token may return 400 at the gateway (nginx)
  with an HTML body; we do not treat 400 as an auth failure since it is a client
  error, not an auth rejection.
- **No API key needed for marketplace.** The earlier assumption that marketplace
  required the Deploy-API-style `Mendix-username` + `Mendix-ApiKey` scheme was wrong.
  PAT is sufficient for both Content API and marketplace.

`internal/auth/scheme.go` host map now lists:

```go
"marketplace-api.mendix.com": SchemePAT,
"catalog.mendix.com":         SchemePAT,
```

API key (`SchemeAPIKey`) is still reserved for the Deploy API follow-up work;
marketplace no longer needs it.

## Mendix Authentication Schemes

Based on current Mendix documentation (2026-04):

| Scheme | Header(s) | APIs | Obtained From |
|---|---|---|---|
| **PAT** (Personal Access Token) | `Authorization: MxToken <pat>` | Content API (marketplace/modules), Platform APIs | Mendix portal → Developer Settings → Personal Access Tokens |
| **API Key** | `Mendix-username: <email>`<br>`Mendix-ApiKey: <key>` | Deploy API, Team Server | Mendix portal → [Profile → API Keys](https://docs.mendix.com/portal/user-settings/#profile-api-keys) |

References:
- Content API / marketplace: <https://docs.mendix.com/apidocs-mxsdk/apidocs/content-api/> (PAT)
- Deploy API: requires API key per <https://docs.mendix.com/portal/user-settings/#profile-api-keys>

API keys "allow apps using them to act on behalf of the user who created the key" with the same privileges. Revocation may take time to propagate due to caching.

### Azure AD / Entra ID — future consideration

Mendix's portal login uses Azure AD SSO. This does **not** mean third-party CLIs can authenticate against AAD directly — doing so requires a public AAD app registration that Mendix has authorized for CLI use, and no such app ID is publicly documented today. For v1, the realistic flow is:

1. User signs into the portal (AAD SSO in the browser, Mendix's problem)
2. User creates a PAT or API key in the portal
3. User pastes the token into `mxcli auth login`

An AAD device-code flow (`mxcli login` opens a browser, no token pasting) remains a Phase 5 stretch goal contingent on Mendix publishing a CLI client ID. The package is designed so this can drop in later without breaking callers.

## Design

### Package Layout

```
internal/auth/
├── credential.go       # Credential struct, Scheme enum
├── scheme.go           # host → scheme mapping
├── resolver.go         # Env vars → keychain → file priority
├── client.go           # *http.Client factory that injects the right headers
├── store.go            # store interface
├── store_file.go       # ~/.mxcli/auth.json (chmod 0600), always available
├── store_keyring.go    # OS keychain via zalando/go-keyring (pure Go, no CGO)
└── errors.go           # ErrUnauthenticated, ErrNoCredential, ErrSchemeMismatch

cmd/mxcli/
└── auth.go             # login / logout / status / list subcommands
```

### Credential Model

```go
package auth

type Scheme string

const (
    SchemePAT    Scheme = "pat"     // MxToken header
    SchemeAPIKey Scheme = "apikey"  // Mendix-username + Mendix-ApiKey
)

type Credential struct {
    Profile   string    `json:"profile"`              // "default", "deploy-ci", ...
    Scheme    Scheme    `json:"scheme"`
    username  string    `json:"username,omitempty"`   // required for apikey
    token     string    `json:"token"`                // never logged
    CreatedAt time.Time `json:"created_at"`
    label     string    `json:"label,omitempty"`      // free-form note
}
```

**Named profiles from day one.** Users with multiple Mendix tenants, or who need separate personal vs. CI credentials, need profile support, and retrofitting it is painful. Default profile name is `default`.

A single profile holds **one** credential. Users who need both a PAT (marketplace) and an API key (deploy) under the same identity create two profiles (`--profile deploy`) — or we store them as a pair under one profile if field testing shows that's common. Starting with one-credential-per-profile keeps the model simple.

### Scheme Routing (Host → Scheme)

Callers shouldn't know which header to set. The client picks based on target hostname:

```go
// internal/auth/scheme.go
var hostSchemes = map[string]Scheme{
    "appstore.home.mendix.com": SchemePAT,
    "cloud.home.mendix.com":    SchemePAT,
    "deploy.mendix.com":        SchemeAPIKey,
    // ...add as new APIs are integrated
}

func schemeForHost(host string) (Scheme, bool) {
    s, ok := hostSchemes[host]
    return s, ok
}
```

If a request targets a host whose scheme doesn't match the resolved credential's scheme, the client returns `ErrSchemeMismatch` with a hint: "host deploy.mendix.com needs an API key; profile 'default' has a PAT. Run `mxcli auth login --profile deploy --api-key`."

### Authenticated HTTP Client

```go
// internal/auth/client.go
func ClientFor(ctx context.Context, profile string) (*http.Client, error) {
    cred, err := Resolve(ctx, profile)
    if err != nil {
        return nil, err
    }
    return &http.Client{
        Transport: &authTransport{cred: cred, inner: http.DefaultTransport},
        timeout:   30 * time.Second,
    }, nil
}

type authTransport struct {
    cred  *Credential
    inner http.RoundTripper
}

func (t *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
    scheme, known := schemeForHost(req.URL.Host)
    if !known {
        return nil, fmt.Errorf("unknown Mendix host: %s", req.URL.Host)
    }
    if scheme != t.cred.Scheme {
        return nil, &ErrSchemeMismatch{host: req.URL.Host, Need: scheme, Have: t.cred.Scheme}
    }
    req = req.Clone(req.Context())
    switch scheme {
    case SchemePAT:
        req.Header.Set("Authorization", "MxToken "+t.cred.Token)
    case SchemeAPIKey:
        req.Header.Set("Mendix-username", t.cred.Username)
        req.Header.Set("Mendix-ApiKey", t.cred.Token)
    }
    resp, err := t.inner.RoundTrip(req)
    if resp != nil && resp.StatusCode == 401 {
        // Wrap as typed error so callers can show a helpful hint
        return resp, &ErrUnauthenticated{Profile: t.cred.Profile}
    }
    return resp, err
}
```

### Storage Priority

The resolver walks these in order, first match wins:

1. **Environment variables** (highest priority, for CI):
   - `MENDIX_PAT` → PAT credential
   - `MENDIX_USERNAME` + `MENDIX_API_KEY` → API key credential
   - `MXCLI_PROFILE` selects which profile env vars populate (default: `default`)
2. **OS keychain** via `github.com/zalando/go-keyring` (pure Go, no CGO — matches CLAUDE.md). Service name `mxcli`, account = profile name. Works on macOS Keychain, Linux Secret Service, Windows Credential Manager.
3. **File fallback** at `~/.mxcli/auth.json`, `chmod 0600`. Necessary for environments without a keyring backend (devcontainers, many CI runners).

Users can force the file backend with `mxcli auth login --no-keychain` — useful for shared devcontainers.

File format (supports multiple profiles):

```json
{
  "version": 1,
  "profiles": {
    "default": {
      "scheme": "pat",
      "token": "xxxxxxxx…",
      "created_at": "2026-04-14T12:00:00Z"
    },
    "deploy-ci": {
      "scheme": "apikey",
      "username": "ci@company.com",
      "token": "xxxxxxxx…",
      "created_at": "2026-04-14T12:05:00Z"
    }
  }
}
```

### CLI Commands

```
mxcli auth login    [--pat | --api-key] [--profile NAME] [--no-keychain]
                    [--token TOKEN] [--username EMAIL]     # non-interactive
mxcli auth logout   [--profile NAME] [--all]
mxcli auth status   [--profile NAME] [--json]
mxcli auth list                                             # all profiles
```

#### `mxcli auth login` (interactive)

```
$ mxcli auth login
Mendix authentication scheme:
  [1] Personal access token (PAT) — recommended for marketplace, content api
  [2] api key                      — required for Deploy api, Team Server
Choice [1]: 2

Email: user@company.com
api key: ****************************
Validating... ✓ authenticated as user@company.com
store in: [1] OS keychain  [2] ~/.mxcli/auth.json
Choice [1]: 1
Saved credential to profile 'default' (scheme: apikey)
```

- Token is read with `golang.org/x/term` (no echo).
- Validation hits a cheap, idempotent authenticated endpoint on the scheme's primary host, e.g. `get https://deploy.mendix.com/api/1/apps` (just to verify the credential works, discarding the response). The exact validation endpoint per scheme is picked during implementation — anything that returns 200 on success and 401 on bad creds.
- Validation failures do not store the credential.

#### `mxcli auth login` (non-interactive, for CI)

```bash
mxcli auth login --pat --token "$MENDIX_PAT" --profile default
mxcli auth login --api-key --username ci@company.com --token "$MENDIX_API_KEY" --profile deploy
```

Or skip `login` entirely and rely on `MENDIX_PAT` / `MENDIX_USERNAME`+`MENDIX_API_KEY` env vars — the resolver picks them up without any stored credential.

#### `mxcli auth status`

```
$ mxcli auth status
Profile:    default
Scheme:     pat
source:     keychain
created:    2026-04-14 12:00:00 UTC
Identity:   user@company.com (verified 2s ago)
```

With `--json` for scripting. Performs a live credential check unless `--offline` is passed.

### Consumer Example (Marketplace)

```go
// cmd/mxcli/cmd_marketplace.go
func installModule(ctx context.Context, componentID string) error {
    client, err := auth.ClientFor(ctx, auth.ProfileFromEnv())
    if err != nil {
        return fmt.Errorf("not authenticated: %w\nrun: mxcli auth login", err)
    }

    url := fmt.Sprintf("https://appstore.home.mendix.com/rest/packagesapi/v2/packages/%s", componentID)
    resp, err := client.Get(url)
    // ...
}
```

No consumer code knows about PATs, API keys, env vars, keychains, or header names.

## Phased Rollout

| Phase | Deliverable | Unblocks |
|---|---|---|
| **1** | `internal/auth` package: Credential, file store, env-var resolver, Authenticator with scheme routing. Keychain store is optional (file fallback covers all environments). | Everything downstream |
| **1.5** | Discovery spike: validate that `appstore.home.mendix.com` accepts `MxToken` PATs as documented (and that `deploy.mendix.com` accepts `Mendix-ApiKey`). One day, parallel to Phase 1. | De-risks Phase 3/4 |
| **2** | `mxcli auth login/logout/status/list` commands. Interactive scheme selection, validation ping before store. | Users can authenticate |
| **3** | Keychain store (`zalando/go-keyring`), prompt-based migration from file to keychain. | Better security on laptops |
| **4** | Wire into `mxcli marketplace` commands (see `PROPOSAL_marketplace_modules.md` Phase 3). | Marketplace install works |
| **5** | Wire into Deploy API (`mxcli deploy` — separate future proposal). | App deployment works |
| **6** (stretch) | AAD device-code flow via MSAL Go, contingent on Mendix publishing a public CLI client ID. Drop-in replacement for the PAT-paste flow in `login`. | `mxcli auth login` without leaving the terminal |

Phases 1 and 2 are independent of the marketplace/deploy features and can ship first. The `internal/auth` package design does not change if Phase 6 later adds AAD — AAD tokens are still credentials, stored with the same profile mechanism; only the acquisition path is new.

## Security Considerations

- **File storage**: `chmod 0600` on create; refuse to read if permissions are more permissive (warn the user to `chmod 0600` the file).
- **Token redaction**: Tokens must never appear in logs, `status` output, `--verbose` traces, or error messages. `Credential.String()` returns `"<scheme> token=REDACTED"`.
- **Process env leakage**: When a credential comes from `MENDIX_PAT`, do not shell out with the env var inherited unless the child process is a trusted Mendix tool (`mx`). Wrap `exec.Command` callers to scrub these vars by default.
- **Revocation caching**: Mendix notes that API key revocation may take time to propagate. `auth status` should surface this ("credential may be cached upstream; revoke in portal for immediate effect") so users aren't surprised.
- **No telemetry**: The auth package does not emit session logs containing credentials or user identity. Only anonymized metrics (success/failure counts) if anything.

## Open Questions

1. **One credential per profile, or both PAT + API key per profile?** One-per-profile is simpler; real usage will tell us if users routinely juggle both under one identity. Revisit after Phase 4.
2. **Project-local profile override?** A `./.mxcli/config.yaml` with `auth_profile: deploy-ci` lets teams pin project-to-profile without env vars. Defer until we see the need.
3. **Credential rotation UX.** `mxcli auth rotate` that walks the user through revoking an old PAT and generating a new one via the portal. Nice-to-have, not v1.
4. **AAD client ID.** Ask Mendix DevRel whether a public CLI app registration exists or is planned. If yes, Phase 6 becomes a real v1 feature; if no, we ship without it and users generate PATs in the portal (the status quo).
5. **Encrypted file backend?** If keychain isn't available, should the file backend encrypt at rest (e.g., age/libsodium with a passphrase)? Adds complexity; most CLIs (`gh`, `doctl`, `az`) rely on filesystem perms alone. Defer.

## References

- Content API authentication: <https://docs.mendix.com/apidocs-mxsdk/apidocs/content-api/>
- Profile API keys: <https://docs.mendix.com/portal/user-settings/#profile-api-keys>
- Marketplace modules proposal: `PROPOSAL_marketplace_modules.md`
- RFC 8628 (device authorization grant, relevant for Phase 6): <https://datatracker.ietf.org/doc/html/rfc8628>
- `zalando/go-keyring`: <https://github.com/zalando/go-keyring>
