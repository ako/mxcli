---
title: mxcli catalog — Mendix Catalog Integration
status: draft
date: 2026-04-16
---

# Proposal: `mxcli catalog` — Mendix Catalog Integration

**Status:** Draft  
**Date:** 2026-04-16  
**Author:** Generated with Claude Code  

**⚠️ TERMINOLOGY NOTE:** This proposal covers the **external Mendix Catalog service** at catalog.mendix.com (CLI: `mxcli catalog search`), which is separate from the **MDL CATALOG keyword** used for querying local project metadata (`SELECT ... FROM CATALOG.entities`). The two concepts are unrelated despite sharing the name "catalog".

## Problem

Mendix Catalog (catalog.mendix.com) is the centralized registry for discovering data sources and services across an organization's landscape. It indexes OData services, REST APIs, SOAP services, and Business Events published by Mendix applications and external systems.

Currently, users must:
1. **Manually browse the Catalog web portal** to discover available services before consuming them
2. **Copy service URLs and metadata paths** into Studio Pro or MDL scripts by hand
3. **Lack scriptable access** for CI/CD pipelines that need to validate service availability or generate reports

This creates friction in two areas:

1. **Development workflow** — Developers waste time navigating the portal UI to find services. No quick CLI lookup exists for "what customer data services are available in Production?"
2. **Automation gaps** — CI/CD pipelines cannot query the Catalog programmatically to validate that a required service exists before attempting to deploy a consuming application.

### Future Goal (Out of Scope for This PR)

Once discovery is implemented, a natural follow-up is **automatic OData client generation from Catalog entries**:

```bash
mxcli catalog create-odata-client <endpoint-uuid> --into MyModule
```

This would fetch metadata from the Catalog-registered endpoint and execute `CREATE EXTERNAL ENTITIES` automatically. However, this proposal focuses on **Phase 1: search and endpoint inspection** to unblock manual workflows first. Client generation is deferred to Phase 2 pending architecture discussion.

## API Discovery

**Base URL:** `https://catalog.mendix.com/rest/search/v5`

**OpenAPI Spec:** <https://docs.mendix.com/openapi-spec/catalog-search_v5.yaml>

**Auth:** `Authorization: MxToken <pat>` (same as marketplace-api.mendix.com). PATs are created at <https://user-settings.mendix.com/> (Developer Settings → Personal Access Tokens).

**Host Whitelisting:** `catalog.mendix.com` is already in `internal/auth/scheme.go` hostSchemes map since the platform auth spike (2026-04-14). No auth infrastructure changes needed.

### Key Endpoints

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/data` | GET | Search catalog with filters (query, serviceType, environment, ownership) |
| `/endpoints/{EndpointUUID}` | GET | Retrieve detailed endpoint metadata (entities, actions, contract) |

### GET /data — Search Endpoint

**Query Parameters:**

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `query` | string | No* | - | Search term (min 3 alphanumeric chars). *Spec says "Yes" but API returns all results if omitted. |
| `serviceType` | string | No | all | Filter by protocol: `OData`, `REST`, `SOAP` |
| `productionEndpointsOnly` | boolean | No | false | Show only Production environments |
| `ownedContentOnly` | boolean | No | false | Show only services where user is business/technical owner |
| `capabilities` | string | No | - | Comma-delimited capabilities (e.g., "updatable"). Combined with AND. |
| `limit` | integer | No | 20 | Results per page (max: 100) |
| `offset` | integer | No | 0 | Zero-based pagination offset |

**Response Schema (200 OK):**

```jsonc
{
  "data": [
    {
      "uuid": "a7f3c2d1-4b5e-6c7f-8d9e-0a1b2c3d4e5f",
      "name": "CustomerService",
      "version": "1.2.0",
      "description": "Manages customer data and relationships",
      "serviceType": "OData",  // or "REST", "SOAP", "Business Events"
      "environment": {
        "name": "Production",
        "location": "EU",
        "type": "Production",  // or "Acceptance", "Test"
        "uuid": "..."
      },
      "application": {
        "name": "CRM Application",
        "description": "...",
        "uuid": "...",
        "businessOwner": "...",
        "technicalOwner": "..."
      },
      "securityClassification": "Public",  // or "Internal", "Confidential"
      "validated": true,
      "lastUpdated": "2026-04-10T14:32:00Z",
      "tags": ["customer", "crm"],
      "entities": [...],  // OData only: array of entity metadata
      "actions": [...]    // OData only: array of action metadata
    }
  ],
  "totalResults": 42,
  "limit": 20,
  "offset": 0,
  "links": [...]  // pagination links
}
```

### GET /endpoints/{EndpointUUID} — Endpoint Details

Returns full metadata for a single endpoint, including:
- Complete entity/action definitions (OData)
- Resource paths and operations (REST)
- WSDL reference (SOAP)
- Contract details (Business Events)

**Critical detail:** The metadata content is **embedded in the response JSON**, not at a separate URL:

```json
{
  "uuid": "9e26c386-9316-4a33-9963-8fe9f69a5117",
  "serviceVersion": {
    "contracts": [
      {
        "type": "CSDL",
        "specificationVersion": "3.0",
        "documents": [
          {
            "isPrimary": true,
            "uri": "metadata.xml",
            "contents": "<?xml version=\"1.0\" encoding=\"utf-8\"?><edmx:Edmx Version=\"1.0\" ...>...</edmx:Edmx>"
          }
        ]
      }
    ],
    "entities": [...],
    "actions": [...]
  }
}
```

The `serviceVersion.contracts[0].documents[0].contents` field contains the complete XML metadata (for OData) or OpenAPI spec (for REST). This means:
- Users **cannot** simply copy a URL and paste into MDL (metadata requires auth to fetch)
- Any "create client from Catalog" feature must handle the API call, either in CLI or executor
- The metadata can be extracted and written to a local file for MDL consumption

**Future PR will use this for `mxcli catalog show <uuid>` and client creation commands.**

## Proposed Command Interface

### `mxcli catalog search <query> [flags]`

**Synopsis:**
```bash
mxcli catalog search <query> [flags]
```

**Arguments:**
- `<query>` — Required positional argument. Search term (min 3 chars recommended by API).

**Flags:**
- `--profile <name>` — Auth profile (default: "default")
- `--service-type <type>` — Filter by protocol: `OData`, `REST`, `SOAP`
- `--production-only` — Show only Production environment endpoints
- `--owned-only` — Show only services where user is owner
- `--limit <n>` — Results per page (default: 20, max: 100)
- `--offset <n>` — Pagination offset (default: 0)
- `--json` — Output as JSON array instead of table

**Examples:**

```bash
# Authenticate first (one-time)
mxcli auth login

# Basic search
mxcli catalog search "customer"

# Filter by service type
mxcli catalog search "customer" --service-type OData

# Production endpoints only
mxcli catalog search "inventory" --production-only

# JSON output for scripting
mxcli catalog search "order" --json | jq '.[] | {name, uuid, type}'

# Pagination
mxcli catalog search "api" --limit 10 --offset 20

# Owned services only
mxcli catalog search "sales" --owned-only
```

### Table Output Format

**Design Decision:** 7 columns, ~155 chars wide with full UUIDs.

```
NAME                TYPE   VERSION  APPLICATION           ENVIRONMENT   PROD  UUID
CustomerService     OData  1.2.0    CRM Application       Production    Yes   a7f3c2d1-4b5e-6c7f-8d9e-0a1b2c3d4e5f
OrderAPI            REST   2.0.1    E-commerce Platform   Acceptance    No    b8e4d3e2-1a2b-3c4d-5e6f-7a8b9c0d1e2f
InventorySync       SOAP   1.0.0    Warehouse System      Test          No    c9f5e4f3-2b3c-4d5e-6f7a-8b9c0d1e2f3a
```

**Column Widths:**
- NAME (22 chars) — Truncate with "..." if longer
- TYPE (8 chars) — OData, REST, SOAP, BusEvt
- VERSION (10 chars) — As returned by API
- APPLICATION (20 chars) — Truncate with "..." if longer
- ENVIRONMENT (12 chars) — Type field (Production, Acceptance, Test)
- PROD (4 chars) — "Yes" if environment.Type == "Production", blank otherwise
- UUID (36 chars) — Full UUID for use with `mxcli catalog show <uuid>`

**Rationale:**
- Full UUIDs required for `catalog show` command (API requires full UUID)
- PROD column provides at-a-glance production status without reading ENVIRONMENT
- APPLICATION provides context without requiring a separate lookup
- Full details available via `--json` for scripting use cases

### JSON Output Format

JSON mode outputs the raw `data` array from the API response:

```jsonc
[
  {
    "uuid": "a7f3c2d1-4b5e-6c7f-8d9e-0a1b2c3d4e5f",
    "name": "CustomerService",
    "version": "1.2.0",
    "serviceType": "OData",
    "environment": { ... },
    "application": { ... },
    // ... all other fields
  },
  // ...
]
```

This enables scripting workflows:

```bash
# Get UUIDs of all production OData services
mxcli catalog search "customer" --service-type OData --production-only --json \
  | jq -r '.[] | .uuid'

# Generate markdown report
mxcli catalog search "api" --json \
  | jq -r '.[] | "- [\(.name)](\(.application.name)) - \(.description)"'
```

### `mxcli catalog show <uuid> [flags]`

**Synopsis:**
```bash
mxcli catalog show <uuid> [flags]
```

**Arguments:**
- `<uuid>` — Required endpoint UUID (from search results)

**Flags:**
- `--profile <name>` — Auth profile (default: "default")
- `--json` — Output full JSON response including embedded contract

**Examples:**

```bash
# Show endpoint details (human-readable)
mxcli catalog show a7f3c2d1-4b5e-6c7f-8d9e-0a1b2c3d4e5f

# JSON output with full contract
mxcli catalog show a7f3c2d1 --json | jq '.serviceVersion.contracts[0].documents[0].contents'
```

**Human-Readable Output:**

```
Name:         CustomerService
Type:         OData
Version:      1.2.0
Application:  CRM Application
Environment:  Production (EU)
Location:     https://crm.acme.com/odata/customer/v1
Description:  Manages customer data and relationships

Security:     Basic, MxID
Validated:    Yes
Last Updated: 2026-04-10T14:32:00Z

Entities (3):
  - Customer (6 attributes, 2 associations)
    Attributes: Name, Email, Phone, Address, City, PostalCode
    Associations: Customer_Order, Customer_Address
  - Order (5 attributes, 1 association)
  - Address (4 attributes)

Actions (2):
  - CalculateDiscount (parameters: CustomerId, DiscountCode)
  - ValidateCustomer (parameters: Email)
```

**JSON Output:**

Returns the complete `/endpoints/{uuid}` API response, including:
- Full endpoint metadata
- Embedded contract (`serviceVersion.contracts[0].documents[0].contents`)
- Entity and action details
- Security scheme
- Application and environment metadata

## Implementation Plan

### 1. File Structure

Create three new files following the existing `internal/auth` + `cmd/mxcli/cmd_*.go` pattern:

```
internal/catalog/types.go       # API request/response structs
internal/catalog/client.go      # HTTP client wrapping catalog.mendix.com/rest/search/v5
cmd/mxcli/cmd_catalog.go        # Cobra commands and RunE handlers
```

### 2. API Types (`internal/catalog/types.go`)

```go
package catalog

type SearchOptions struct {
    Query                   string
    ServiceType             string // "OData", "REST", "SOAP", "" (all)
    ProductionEndpointsOnly bool
    OwnedContentOnly        bool
    Limit                   int
    Offset                  int
}

type SearchResponse struct {
    Data         []SearchResult `json:"data"`
    TotalResults int            `json:"totalResults"`
    Limit        int            `json:"limit"`
    Offset       int            `json:"offset"`
}

type SearchResult struct {
    UUID                   string      `json:"uuid"`
    Name                   string      `json:"name"`
    Version                string      `json:"version"`
    Description            string      `json:"description"`
    ServiceType            string      `json:"serviceType"`
    Environment            Environment `json:"environment"`
    Application            Application `json:"application"`
    SecurityClassification string      `json:"securityClassification"`
    Validated              bool        `json:"validated"`
    LastUpdated            string      `json:"lastUpdated"`
}

type Environment struct {
    Name     string `json:"name"`
    Location string `json:"location"`
    Type     string `json:"type"` // "Production", "Acceptance", "Test"
    UUID     string `json:"uuid"`
}

type Application struct {
    Name             string `json:"name"`
    Description      string `json:"description"`
    UUID             string `json:"uuid"`
    BusinessOwner    string `json:"businessOwner"`
    TechnicalOwner   string `json:"technicalOwner"`
}
```

### 3. HTTP Client (`internal/catalog/client.go`)

**Pattern:** Reuse `internal/auth/client.go` authentication transport.

```go
package catalog

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "net/url"
    "strconv"
    
    "github.com/mendixlabs/mxcli/internal/auth"
)

const baseURL = "https://catalog.mendix.com/rest/search/v5"

type Client struct {
    httpClient *http.Client
    baseURL    string
}

func NewClient(ctx context.Context, profile string) (*Client, error) {
    httpClient, err := auth.ClientFor(ctx, profile)
    if err != nil {
        return nil, err
    }
    return &Client{
        httpClient: httpClient,
        baseURL:    baseURL,
    }, nil
}

func (c *Client) Search(ctx context.Context, opts SearchOptions) (*SearchResponse, error) {
    // Build query params
    params := url.Values{}
    if opts.Query != "" {
        params.Set("query", opts.Query)
    }
    if opts.ServiceType != "" {
        params.Set("serviceType", opts.ServiceType)
    }
    if opts.ProductionEndpointsOnly {
        params.Set("productionEndpointsOnly", "true")
    }
    if opts.OwnedContentOnly {
        params.Set("ownedContentOnly", "true")
    }
    if opts.Limit > 0 {
        params.Set("limit", strconv.Itoa(opts.Limit))
    }
    if opts.Offset > 0 {
        params.Set("offset", strconv.Itoa(opts.Offset))
    }
    
    // Make request
    reqURL := c.baseURL + "/data?" + params.Encode()
    req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
    if err != nil {
        return nil, err
    }
    req.Header.Set("Accept", "application/json")
    
    resp, err := c.httpClient.Do(req)
    if err != nil {
        // auth.authTransport wraps 401/403 as auth.ErrUnauthenticated
        return nil, err
    }
    defer resp.Body.Close()
    
    if resp.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("catalog API returned status %d", resp.StatusCode)
    }
    
    var result SearchResponse
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return nil, fmt.Errorf("decode response: %w", err)
    }
    
    return &result, nil
}
```

**Error Handling:**
- `auth.ClientFor()` returns `auth.ErrNoCredential` if no PAT is stored → command wraps with "Run: mxcli auth login"
- `auth.authTransport` wraps 401/403 as `auth.ErrUnauthenticated` → command wraps with "Authentication failed. Run: mxcli auth login"
- Non-200 responses return status code → command wraps with "Catalog API error"

### 4. CLI Command (`cmd/mxcli/cmd_catalog.go`)

**Pattern:** Follow `cmd/mxcli/cmd_auth.go` structure (Cobra command, flags, tabwriter/JSON output).

```go
package main

import (
    "encoding/json"
    "fmt"
    "strings"
    "text/tabwriter"
    
    "github.com/mendixlabs/mxcli/internal/auth"
    "github.com/mendixlabs/mxcli/internal/catalog"
    "github.com/spf13/cobra"
)

var catalogCmd = &cobra.Command{
    Use:   "catalog",
    Short: "Search and manage Mendix Catalog services",
    Long: `Search for data sources and services registered in Mendix Catalog.

Requires authentication via Personal Access Token (PAT). Create a PAT at:
  https://user-settings.mendix.com/

Storage priority:
  1. MENDIX_PAT env var (set MXCLI_PROFILE to target a non-default profile)
  2. ~/.mxcli/auth.json (mode 0600)`,
}

var catalogSearchCmd = &cobra.Command{
    Use:   "search <query>",
    Short: "Search for services in the Catalog",
    Long: `Search for data sources and services in Mendix Catalog.

Examples:
  mxcli catalog search "customer"
  mxcli catalog search "order" --service-type OData
  mxcli catalog search "api" --production-only --json`,
    Args: cobra.ExactArgs(1),
    RunE: runCatalogSearch,
}

func init() {
    catalogSearchCmd.Flags().String("profile", auth.ProfileDefault, "credential profile name")
    catalogSearchCmd.Flags().String("service-type", "", "filter by service type (OData, REST, SOAP)")
    catalogSearchCmd.Flags().Bool("production-only", false, "show only production endpoints")
    catalogSearchCmd.Flags().Bool("owned-only", false, "show only owned services")
    catalogSearchCmd.Flags().Int("limit", 20, "results per page (max 100)")
    catalogSearchCmd.Flags().Int("offset", 0, "pagination offset")
    catalogSearchCmd.Flags().Bool("json", false, "output as JSON array")
    
    catalogCmd.AddCommand(catalogSearchCmd)
    rootCmd.AddCommand(catalogCmd)
}

func runCatalogSearch(cmd *cobra.Command, args []string) error {
    query := args[0]
    profile, _ := cmd.Flags().GetString("profile")
    serviceType, _ := cmd.Flags().GetString("service-type")
    prodOnly, _ := cmd.Flags().GetBool("production-only")
    ownedOnly, _ := cmd.Flags().GetBool("owned-only")
    limit, _ := cmd.Flags().GetInt("limit")
    offset, _ := cmd.Flags().GetInt("offset")
    asJSON, _ := cmd.Flags().GetBool("json")
    
    // Create client
    client, err := catalog.NewClient(cmd.Context(), profile)
    if err != nil {
        if _, ok := err.(*auth.ErrNoCredential); ok {
            return fmt.Errorf("no credential found. Run: mxcli auth login")
        }
        return err
    }
    
    // Execute search
    opts := catalog.SearchOptions{
        Query:                   query,
        ServiceType:             serviceType,
        ProductionEndpointsOnly: prodOnly,
        OwnedContentOnly:        ownedOnly,
        Limit:                   limit,
        Offset:                  offset,
    }
    resp, err := client.Search(cmd.Context(), opts)
    if err != nil {
        if _, ok := err.(*auth.ErrUnauthenticated); ok {
            return fmt.Errorf("authentication failed. Run: mxcli auth login")
        }
        return err
    }
    
    // Output
    if asJSON {
        return outputJSON(cmd, resp.Data)
    }
    return outputTable(cmd, resp)
}

func outputTable(cmd *cobra.Command, resp *catalog.SearchResponse) error {
    if len(resp.Data) == 0 {
        fmt.Fprintln(cmd.OutOrStdout(), "No results found.")
        return nil
    }
    
    w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
    fmt.Fprintln(w, "NAME\tTYPE\tVERSION\tAPPLICATION\tENVIRONMENT\tPROD\tUUID")
    
    for _, item := range resp.Data {
        name := truncate(item.Name, 22)
        typ := truncate(item.ServiceType, 8)
        version := truncate(item.Version, 10)
        app := truncate(item.Application.Name, 20)
        env := truncate(item.Environment.Type, 12)
        prod := ""
        if item.Environment.Type == "Production" {
            prod = "Yes"
        }
        uuid := item.UUID[:8] // Short UUID
        
        fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
            name, typ, version, app, env, prod, uuid)
    }
    
    fmt.Fprintf(w, "\nTotal: %d results (showing %d-%d)\n",
        resp.TotalResults, resp.Offset+1, resp.Offset+len(resp.Data))
    
    return w.Flush()
}

func outputJSON(cmd *cobra.Command, data []catalog.SearchResult) error {
    enc := json.NewEncoder(cmd.OutOrStdout())
    enc.SetIndent("", "  ")
    return enc.Encode(data)
}

func truncate(s string, max int) string {
    if len(s) <= max {
        return s
    }
    return s[:max-3] + "..."
}
```

### 5. Testing Strategy

**Unit Tests:**

1. **`internal/catalog/types_test.go`** — JSON unmarshaling
   - Valid API response → SearchResponse struct
   - Missing optional fields handled gracefully
   
2. **`internal/catalog/client_test.go`** — HTTP client
   - `httptest.Server` mocking API responses
   - Query param encoding
   - Error handling (401, 403, 500, network errors)
   
3. **`cmd/mxcli/cmd_catalog_test.go`** — Command integration
   - Flag parsing
   - Table output formatting
   - JSON output formatting
   - Error message wrapping

**Manual Testing:**

```bash
# Prerequisites
mxcli auth login

# Basic search
mxcli catalog search "customer"

# Filters
mxcli catalog search "api" --service-type OData
mxcli catalog search "data" --production-only
mxcli catalog search "service" --owned-only

# Pagination
mxcli catalog search "test" --limit 5
mxcli catalog search "test" --limit 5 --offset 5

# JSON output
mxcli catalog search "order" --json | jq '.[] | .name'

# Error cases
mxcli auth logout
mxcli catalog search "test"  # Should error: "Run: mxcli auth login"
```

## Trade-offs & Design Decisions

### 1. Query Parameter: Required or Optional?

**API Spec:** Marks `query` as required, but actual API returns all results if omitted.

**Decision:** Make query a **required positional argument** in CLI for clarity:
- `mxcli catalog search "customer"` is intuitive
- Forces users to think about what they're searching for (avoids accidental full-catalog dumps)
- Matches common CLI patterns (grep, gh, curl)

**Alternative:** Make query optional and default to empty (list all). Rejected because:
- Returning 1000+ results by default is poor UX
- Users can specify `--limit 100` if they want a large result set

### 2. UUID Display: Full or Short?

**Decision:** Short (8 chars) in table, full in JSON.

**Rationale:**
- 36-char UUIDs consume significant column width
- First 8 chars are usually unique enough for manual lookup
- Users needing full UUIDs for automation can use `--json` mode
- Matches common CLI patterns (git log --oneline, docker ps)

### 3. Table vs. JSON Default Output

**Decision:** Table by default, JSON via `--json` flag.

**Rationale:**
- Human users expect readable tables (matches `mxcli auth status`, `gh pr list`)
- Automation uses `--json` (explicit opt-in prevents breaking changes to table format)
- JSON output includes all fields; table shows curated subset

### 4. No Caching

**Decision:** No client-side caching. Every command makes a fresh API call.

**Rationale:**
- Catalog data changes frequently (new deployments, environment changes)
- Stale cache could mislead users about service availability
- API response times are acceptable (<500ms observed)
- Caching adds complexity (TTL, invalidation, storage)

**Future:** If performance becomes an issue, add opt-in cache (`--cache` flag with TTL).

### 5. Pagination: Manual vs. Auto

**Decision:** Expose `--limit` and `--offset` flags. No auto-pagination.

**Rationale:**
- Simple implementation (no "press any key for next page" logic)
- Predictable for scripting (no interactive prompts in CI)
- Users can pipe to `less` for paging: `mxcli catalog search "api" --limit 100 | less`

**Future:** Add interactive pagination with arrow keys (bubble tea TUI) as a separate mode.

## Architectural Discussion: Client Creation from Catalog

The embedded metadata in `/endpoints/{uuid}` responses creates an architectural question for future client creation features.

### Key Constraint

The Catalog API returns metadata **embedded in JSON**, not at a public URL:
- `serviceVersion.contracts[0].documents[0].contents` contains the full XML/OpenAPI spec
- Metadata requires auth (PAT token) to fetch
- Users cannot simply copy a URL into MDL scripts

### Three Architectural Options

#### Option A: MDL-First (Executor Integration)

Extend MDL grammar to support Catalog references:

```sql
CREATE EXTERNAL ENTITIES FROM CATALOG 'a7f3c2d1-4b5e-6c7f-8d9e-0a1b2c3d4e5f' INTO MyModule;
```

**Flow:** Parser → Executor calls `catalog.Client.GetEndpoint(uuid)` → Extracts metadata XML → Parses → Creates entities

**Pros:** Consistent with MDL philosophy, works in REPL/scripts, single syntax  
**Cons:** Executor becomes network-aware (auth, latency, errors), grammar change, REPL sessions need credentials

#### Option B: CLI Wrapper (No Executor Changes)

CLI command fetches metadata and executes MDL:

```bash
mxcli catalog create-odata-client a7f3c2d1 --into MyModule -p app.mpr
```

**Flow:** CLI → Fetch metadata → Write temp file → Execute `CREATE EXTERNAL ENTITIES FROM '/tmp/...' INTO MyModule`

**Pros:** No executor changes, auth stays in CLI, quick to implement  
**Cons:** Not usable in REPL, two ways to create clients, temp file management

#### Option C: CLI Helper (Metadata Export)

CLI exports metadata, user runs MDL separately:

```bash
mxcli catalog export-metadata a7f3c2d1 --output metadata.xml
# Then in MDL:
CREATE EXTERNAL ENTITIES FROM 'metadata.xml' INTO MyModule;
```

**Flow:** CLI → Fetch → Write user-specified file → User executes MDL

**Pros:** Clear separation, no executor changes, explicit workflow, metadata visible/versionable  
**Cons:** Two-step workflow, manual step, metadata staleness

### Recommendation

**Phase 1 (this PR):** Implement search only, defer architecture decision.

**Phase 2:** Prototype both Option A and Option C to compare UX:
- Option A for integrated workflow (test executor network integration)
- Option C for explicit workflow (test two-step UX)

Choose based on user feedback and implementation complexity.

## Future Enhancements (Out of Scope)

### 1. `mxcli catalog create-odata-client <uuid>`

Generate OData client from Catalog entry:

```bash
mxcli catalog create-odata-client a7f3c2d1 --into MyModule -p app.mpr

# Equivalent to:
# 1. GET /endpoints/a7f3c2d1 → extract metadata URL
# 2. CREATE EXTERNAL ENTITIES FROM 'http://...$metadata' INTO MyModule
```

**Implementation:** Fetch endpoint details, extract metadata content, call existing `CREATE EXTERNAL ENTITIES` executor with embedded metadata.

### 2. Interactive Search UI

TUI with arrow-key navigation and fuzzy search:

```bash
mxcli catalog search --interactive

# Opens bubble tea UI:
┌─────────────────────────────────────────────────────────────┐
│ Search: customer▊                                           │
├─────────────────────────────────────────────────────────────┤
│ > CustomerService (OData 1.2.0) - CRM Application          │
│   CustomerAPI (REST 2.0.1) - E-commerce Platform           │
│   CustomerData (OData 1.0.0) - Legacy System               │
└─────────────────────────────────────────────────────────────┘
Press Enter to view details, Esc to exit
```

**Implementation:** Use [bubble tea](https://github.com/charmbracelet/bubbletea) framework (already used in mxcli TUI). Reuse `catalog.Client.Search()`.

## CHANGELOG Entry

```markdown
### Added

- **mxcli catalog search** — Search Mendix Catalog for data sources and services with filters for service type, environment, and ownership (#XXX)
```

## References

- OpenAPI Spec: <https://docs.mendix.com/openapi-spec/catalog-search_v5.yaml>
- Mendix Catalog Docs: <https://docs.mendix.com/catalog/>
- Platform Auth Proposal: `docs/11-proposals/PROPOSAL_platform_auth.md`
- Auth Implementation: `internal/auth/`, `cmd/mxcli/cmd_auth.go`
