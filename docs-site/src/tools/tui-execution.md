# MDL Execution

The TUI includes a built-in MDL execution view for running MDL statements directly from the terminal interface. Press `x` to open the execution view.

## Opening the Execution View

From the browser mode, press `x` to switch to the MDL execution view. A full-screen textarea appears with line numbers and a placeholder prompt.

## Keybindings

| Key | Action |
|-----|--------|
| `x` | Open the execution view (from browser mode) |
| `Ctrl+E` | Execute the MDL script in the textarea |
| `Ctrl+O` | Open a file picker to load an MDL file |
| `Esc` | Close the execution view and return to the browser |

## Writing and Executing MDL

The textarea supports standard text editing. Type or paste any MDL statements:

```sql
SHOW MODULES;

CREATE PERSISTENT ENTITY MyModule.Customer (
  Name: String(100),
  Email: String(200),
  Age: Integer
);
```

Press `Ctrl+E` to execute. The TUI writes the content to a temporary file and runs `mxcli exec` as a subprocess. During execution, a status indicator shows "Executing..." in the status bar.

## Viewing Results

After execution completes, results appear in a fullscreen overlay with syntax highlighting. The overlay shows:

- The command output for successful executions (e.g., confirmation messages, query results)
- Error details for failed executions, prefixed with `-- Error:`

Press `q` or `Esc` to close the result overlay.

## Tree Refresh

When an MDL script executes successfully (e.g., creating or modifying elements), the project tree automatically refreshes to reflect the changes. This means you can create an entity, close the result overlay, and immediately see it in the browser.

## Loading from File

Press `Ctrl+O` to open a file picker dialog. The TUI attempts to use a native file picker:

- **Linux**: `zenity --file-selection` or `kdialog --getopenfilename`
- **macOS**: Native file dialog

The file picker filters for `.mdl` files. After selecting a file, its content is loaded into the textarea. A status message confirms the loaded file path. You can then review or edit the content before executing with `Ctrl+E`.

If no file picker is available, the TUI displays an informational message suggesting installation of `zenity` or `kdialog`.

## Supported MDL Statements

The execution view accepts any valid MDL statement, including:

| Category | Examples |
|----------|---------|
| **Query** | `SHOW MODULES`, `SHOW ENTITIES`, `DESCRIBE ENTITY MyModule.Customer` |
| **Create** | `CREATE ENTITY`, `CREATE MICROFLOW`, `CREATE PAGE` |
| **Modify** | `ALTER ENTITY`, `ALTER PAGE`, `ALTER NAVIGATION` |
| **Delete** | `DROP ENTITY`, `DROP MICROFLOW`, `DROP PAGE` |
| **Security** | `GRANT`, `REVOKE`, `CREATE USER ROLE` |
| **Catalog** | `REFRESH CATALOG`, `SELECT FROM CATALOG.ENTITIES` |
| **Search** | `SEARCH 'keyword'` |

Multiple statements can be included in a single execution, separated by semicolons.
