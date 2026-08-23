# Claude Code Integration

Claude Code is the primary AI integration for mxcli. It gets the deepest support: a dedicated configuration directory, project-level context in `CLAUDE.md`, skill files that teach Claude MDL patterns, and slash commands for common operations.

## Initializing a project

Claude Code is the default tool, so you don't need to specify a flag:

```bash
mxcli init /path/to/my-mendix-project
```

This is equivalent to:

```bash
mxcli init --tool claude /path/to/my-mendix-project
```

## What gets created

After running `mxcli init`, your project directory gains several new entries:

```
my-mendix-project/
├── CLAUDE.md                    # Project context for Claude Code
├── AGENTS.md                    # Universal AI assistant guide
├── .claude/
│   ├── settings.json            # Claude Code settings
│   ├── commands/                # Slash commands (/create-entity, etc.)
│   └── lint-rules/              # Starlark lint rules
├── .ai-context/
│   ├── skills/                  # MDL pattern guides (shared by all tools)
│   └── examples/                # Example MDL scripts
├── .devcontainer/
│   ├── devcontainer.json        # Dev container configuration
│   └── Dockerfile               # Container image with mxcli, JDK, Docker
├── mxcli                        # CLI binary (copied into project)
└── app.mpr                      # Your Mendix project (already existed)
```

### CLAUDE.md

The `CLAUDE.md` file gives Claude Code project-level context. It describes what mxcli is, lists the available MDL commands, and tells Claude to read the skill files before writing MDL. This file is automatically read by Claude Code when it starts.

### Skills

The `.claude/skills/` directory (and `.ai-context/skills/` for the universal copy) holds one directory per skill, each with a `SKILL.md`. For example, `write-microflows/SKILL.md` explains microflow syntax, common mistakes, and a validation checklist. Claude Code discovers them from `.claude/skills/` automatically and reads the relevant one before generating MDL, which dramatically improves output quality.

### Commands

The `.claude/commands/` directory contains slash commands that you can invoke from within Claude Code. These provide shortcuts for common operations.

## Setting up the dev container

The dev container is the recommended way to work with Claude Code. It sandboxes the AI so it can only access your project files, and it comes pre-configured with everything you need.

### What's installed in the container

| Component | Purpose |
|-----------|---------|
| **mxcli** | Mendix CLI (copied into project root) |
| **MxBuild / mx** | Mendix project validation and building |
| **JDK 21** (Adoptium) | Required by MxBuild |
| **Docker-in-Docker** | Running Mendix apps locally with `mxcli docker` |
| **Node.js** | Playwright testing support |
| **PostgreSQL client** | Database connectivity |
| **Claude Code** | Auto-installed when the container starts |

### Opening the dev container

1. Open your project folder in VS Code
2. VS Code detects the `.devcontainer/` directory and shows a notification
3. Click **"Reopen in Container"** (or use the command palette: `Dev Containers: Reopen in Container`)
4. Wait for the container to build (first time takes a few minutes)
5. Once inside the container, open a terminal

## Starting Claude Code

With the dev container running, open a terminal in VS Code and start Claude:

```bash
claude
```

Claude Code now has access to your project files, the mxcli binary, and all the skill files. You can start asking it to do things.

## How Claude works with your project

Claude follows a consistent pattern when you ask it to modify your Mendix project:

### 1. Explore

Claude uses mxcli commands to understand your project before making changes:

```sql
-- What modules exist?
SHOW MODULES;

-- What entities are in this module?
SHOW ENTITIES IN Sales;

-- What does this entity look like?
DESCRIBE ENTITY Sales.Customer;

-- What microflows exist?
SHOW MICROFLOWS IN Sales;

-- Search for something specific
SEARCH 'validation';
```

### 2. Read the relevant skill

Before writing MDL, Claude reads the appropriate skill file. If you ask for a microflow, it reads `write-microflows.md`. If you ask for a page, it reads `create-page.md`. The skills contain syntax references, examples, and validation checklists.

### 3. Write MDL

Claude generates an MDL script based on what it learned from the project and the skill files:

```sql
/** Customer master data */
@Position(100, 100)
CREATE PERSISTENT ENTITY Sales.Customer (
    Name: String(200) NOT NULL,
    Email: String(200) NOT NULL,
    Phone: String(50),
    IsActive: Boolean DEFAULT true
);
```

### 4. Validate

Claude checks the script for syntax errors and reference issues:

```bash
./mxcli check script.mdl
./mxcli check script.mdl -p app.mpr --references
```

### 5. Execute

If validation passes, Claude runs the script against your project:

```bash
./mxcli -p app.mpr -c "EXECUTE SCRIPT 'script.mdl'"
```

### 6. Verify

Claude can run a full project check using the Mendix build tools:

```bash
./mxcli docker check -p app.mpr
```

## Example interaction

Here is a typical conversation with Claude Code:

**You:** Create a Customer entity in the Sales module with name, email, and phone. Then create an overview page that shows all customers in a data grid.

**Claude** (explores the project):
```bash
./mxcli -p app.mpr -c "SHOW MODULES"
./mxcli -p app.mpr -c "SHOW ENTITIES IN Sales"
./mxcli -p app.mpr -c "SHOW PAGES IN Sales"
```

**Claude** (reads skills, writes MDL, validates, and executes):
```sql
/** Customer contact information */
@Position(100, 100)
CREATE PERSISTENT ENTITY Sales.Customer (
    Name: String(200) NOT NULL,
    Email: String(200) NOT NULL,
    Phone: String(50),
    IsActive: Boolean DEFAULT true,
    CreatedAt: DateTime DEFAULT '[%CurrentDateTime%]'
);

/** Customer overview page with data grid */
CREATE PAGE Sales.Customer_Overview
(
  Title: 'Customers',
  Layout: Atlas_Core.Atlas_Default
)
{
  LAYOUTGRID mainGrid {
    ROW row1 {
      COLUMN col1 (DesktopWidth: 12) {
        DATAGRID dgCustomers (
          DataSource: DATABASE FROM Sales.Customer SORT BY Name ASC
        ) {
          COLUMN colName (Attribute: Name, Caption: 'Name')
          COLUMN colEmail (Attribute: Email, Caption: 'Email')
          COLUMN colPhone (Attribute: Phone, Caption: 'Phone')
          COLUMN colActive (Attribute: IsActive, Caption: 'Active')
        }
      }
    }
  }
}
```

Claude validates the script, executes it, and reports back. You can then open the project in Studio Pro to review the result.

## Tips for working with Claude Code

- **Be specific about module names.** Say "Create a Customer entity in the Sales module" rather than just "Create a Customer entity."
- **Mention existing elements.** If you want an association to an existing entity, name it: "Link Order to the existing Sales.Customer entity."
- **Let Claude explore first.** If you're asking for changes to an existing project, Claude will run SHOW and DESCRIBE commands to understand what's already there. This leads to better results than trying to describe the current state yourself.
- **Review in Studio Pro.** After Claude makes changes, open the project in Studio Pro to verify everything looks right visually.
- **Use `mxcli docker check`** to catch issues that `mxcli check` alone might miss. The Mendix build tools perform deeper validation.

## Next steps

If you use other AI tools alongside Claude Code, see [Other AI tools](other-ai-tools.md) (GitHub Copilot, OpenCode, Cursor, Continue.dev, Windsurf). To understand how skills work in detail, see [Skills and CLAUDE.md](skills.md).
