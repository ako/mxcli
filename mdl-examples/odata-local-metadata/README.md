# OData Local Metadata Example

This example demonstrates how to create consumed OData services using local metadata files instead of fetching from HTTP(S) URLs.

## Use Cases

- **Offline development** — work without network access
- **Testing and CI/CD** — use metadata snapshots for reproducibility
- **Version-pinned metadata** — lock to a specific metadata version
- **Pre-production services** — test against metadata files before deployment

## Important Notes

1. **Relative paths are normalized** — Any relative path is automatically converted to an absolute `file://` URL in the Mendix model for Studio Pro compatibility. Re-running the script from a checkout at another path keeps the stored URL when it names the same project file, and `describe` prints a stored URL inside the project as `./<path>` (ako/mxcli#1499)
2. **ServiceUrl must be a constant** — Always use `@Module.ConstantName` format, not direct URLs

## Supported Formats

### 1. Absolute `file://` URI
```mdl
CREATE CONSUMED ODATA SERVICE MyModule.Service (
  MetadataUrl: 'file:///absolute/path/to/metadata.xml'
);
```

### 2. Relative path (with or without `./`)
```mdl
mdl 1;
-- Resolved relative to the .mpr file's directory, then normalized to absolute file://
-- Example: './metadata/service.xml' → 'file:///absolute/path/to/project/metadata/service.xml'
CREATE CONSUMED ODATA SERVICE MyModule.Service (
  MetadataUrl: './metadata/service.xml',
  ServiceUrl: '@MyModule.ServiceLocation'
);

CREATE CONSUMED ODATA SERVICE MyModule.Service2 (
  MetadataUrl: 'metadata/service.xml',
  ServiceUrl: '@MyModule.ServiceLocation'
);
```

### 3. HTTP(S) URL (existing behavior)
```mdl
CREATE CONSUMED ODATA SERVICE MyModule.Service (
  MetadataUrl: 'https://api.example.com/$metadata'
);
```

## Path Resolution

| Scenario | Base Directory |
|----------|----------------|
| Project loaded (`-p` flag or REPL with project) | Relative to `.mpr` file's directory |
| No project loaded (`mxcli check` without `-p`) | Relative to current working directory |

## Running the Example

```bash
# From the project root
./bin/mxcli exec mdl-examples/odata-local-metadata/example.mdl -p path/to/app.mpr

# Or in REPL
./bin/mxcli -p path/to/app.mpr
> .read mdl-examples/odata-local-metadata/example.mdl
```

## Hash Calculation

Local files are hashed identically to HTTP-fetched metadata (SHA-256). Editing the local XML file invalidates the cached metadata, just like a remote service change would.

## Benefits

- ✅ No network required
- ✅ Reproducible builds
- ✅ Version control friendly (commit metadata alongside code)
- ✅ Firewall-friendly
- ✅ Fast iteration during development
