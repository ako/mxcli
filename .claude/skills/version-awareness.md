# Version Awareness

This skill teaches you to check the project's Mendix version before generating MDL, so you never write syntax the project can't support.

## Before Generating MDL

Always check the project's Mendix version before writing MDL:

```sql
show status;           -- shows connected project version
show features;         -- shows all features with availability for this version
show features in integration;  -- filter by area
```

If you're not connected to a project, query any version directly:

```sql
show features for version 10.24;
```

## Version-Conditional Patterns

If a feature shows "No" in the Available column, **do not use it**. Use the documented workaround instead.

Common version gates:

| Feature | Requires | Workaround for older versions |
|---------|----------|-------------------------------|
| VIEW ENTITY | 10.18+ | Regular entity with microflow data source |
| Page parameters | 11.0+ | Pass data via non-persistent entity |
| REST query params | 11.0+ | Build query string manually in microflow |
| DB runtime connection | 11.0+ | Hardcode connection in Database Connector config |
| Design properties v3 | 11.0+ | Use Atlas v2 design properties |
| Agent Editor documents (MODEL, KNOWLEDGE BASE, CONSUMED MCP SERVICE, AGENT) | 11.9+, plus the AgentEditorCommons module | None — call an LLM from a microflow via the OpenAI Connector instead |

The executor will reject commands that target unavailable features with an actionable error — but checking upfront avoids wasted work.

The agent doctypes are the row where checking upfront matters most: their documents are stored as **custom blobs**, so mxbuild validates nothing about them. On a project below 11.9 the build stays green and the fault appears only when Studio Pro opens the document. Every other row in this table is caught by `mx check` sooner or later; that one is not.

## Upgrade Planning

When migrating to a newer version:

```sql
show features added since 10.24;    -- what's new if upgrading from 10.24
```

## Checklist

Before writing any MDL for a connected project:

1. Run `show status` to confirm the Mendix version
2. If using view entities, page parameters, REST clients, or database queries — run `show features` to verify availability
3. If a feature is unavailable, use the workaround pattern
4. Run `mxcli check script.mdl -p app.mpr --references` to validate before execution
