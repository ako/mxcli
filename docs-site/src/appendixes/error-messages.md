# Error Messages Reference

Common error messages from Studio Pro, mxcli, and the MDL parser, with explanations and solutions.

## Studio Pro Errors

These errors appear when opening a project in Studio Pro after modification by mxcli.

### TypeCacheUnknownTypeException

```
TypeCacheUnknownTypeException: The type cache does not contain a type
with qualified name DomainModels$Index
```

**Cause:** The BSON `$Type` field uses the **qualifiedName** instead of the **storageName**. These are often identical, but not always.

**Solution:** Check the metamodel reflection data for the correct storage name:

| qualifiedName (wrong) | storageName (correct) |
|-----------------------|----------------------|
| `DomainModels$Entity` | `DomainModels$EntityImpl` |
| `DomainModels$Index` | `DomainModels$EntityIndex` |

Look up the type in `reference/mendixmodellib/reflection-data/<version>-structures.json` and use the `storageName` field value.

### CE0463: Widget definition changed

```
CE0463: The widget definition of 'DataGrid2' has changed.
```

**Cause:** The widget's `WidgetObject` properties do not match its `PropertyTypes` schema. This happens when:

- A widget template is missing properties
- Property values have incorrect types
- The template was extracted from a different Mendix version

**Solution:**
1. Create the same widget manually in Studio Pro
2. Extract its BSON from the saved project
3. Compare the template's `object` section against the Studio Pro version
4. Update `sdk/widgets/templates/<version>/<Widget>.json` to match

See the debug workflow in `.claude/skills/debug-bson.md` for step-by-step instructions.

### CE0066: Entity access is out of date

```
CE0066: Entity access for 'MyModule.Customer' is out of date.
```

**Cause:** Association MemberAccess entries were added to the wrong entity. In Mendix, association access rules must only be on the **FROM** entity (the one stored in `ParentPointer`), not the TO entity.

**Solution:** Ensure `MemberAccess` entries for associations are added only to the entity that owns the foreign key (the FROM side of the association). Remove any association MemberAccess entries from the TO entity.

### System.ArgumentNullException (ValidationRule)

```
System.ArgumentNullException: Value cannot be null.
```

**Cause:** A validation rule's `Attribute` field uses a binary UUID instead of a qualified name string. The metamodel specifies `BY_NAME_REFERENCE` for this field.

**Solution:** Use a qualified name string (e.g., `"Module.Entity.Attribute"`) for the `Attribute` field in ValidationRule BSON, not a binary UUID.

## mxcli Check Errors

These rule IDs appear in `mxcli check` output and in LSP diagnostics in VS Code. They fire before MxBuild runs, so most pluggable-widget mistakes are caught at authoring time.

### MDL-WIDGET01: Unknown pluggable widget property

```
page MyModule.OrderList: widget `cb1` (combobox) has no property
`optionsSourcType` — did you mean `optionsSourceType`? [MDL-WIDGET01]
```

**Cause:** The property key written on a pluggable widget is not declared in the widget's `.def.json` (the extracted schema from its `.mpk`). Usually a typo; sometimes a property that exists in a different widget but not this one.

**Solution:**
1. Compare the key against the widget's known properties — `mxcli describe widget <Name>` lists them.
2. Use the suggested replacement if one is offered (Levenshtein-nearest match).
3. If the property genuinely doesn't exist on this widget version, check that `.mxcli/widgets/` has the latest schema: `mxcli refresh catalog -p app.mpr` re-extracts any `.mpk` whose mtime changed.
4. If the property was just added by a `.mpk` upgrade, make sure `mxcli init` or `widget init` was run after the upgrade.

This rule also fires in the LSP — the property key shows as a red squiggle while you type.

### MDL-WIDGET02: Legacy native widget on upgraded project

```
page MyModule.OrderList: widget `OrdersGrid` uses deprecated native
`Forms$DataGrid` (deprecated from Mendix 11.0.0) — migrate to pluggable
Datagrid 2.x (`DATAGRID` keyword resolves to this on 11.0+) (project is
on 11.9.0) [MDL-WIDGET02]
```

**Cause:** Studio Pro does not auto-migrate native-stack widgets (e.g. `Forms$DataGrid`) to their pluggable replacements during a Mendix major-version upgrade. After upgrading from Mendix 10.x to 11.x, your pages can still contain the native widget unless you opened them and migrated by hand.

**Solution:**
1. Run `mxcli check --post-migration -p app.mpr` to get the full list of pages and widgets to migrate.
2. Open each reported page in Studio Pro and replace the native widget with its pluggable equivalent. For DataGrid, this means using the `DATAGRID` MDL keyword (which resolves to pluggable Datagrid 2.x on Mendix 11.0+) or `pluggablewidget 'com.mendix.widget.web.datagrid.Datagrid'`.
3. Re-run the scan to confirm the page is clean.

The deprecated-widget catalog is hand-maintained in `mdl/executor/keyword_dispatch.go` (`LegacyWidgets`). Open an issue if you spot a native widget that should be on the list.

## mxcli Parser Errors

### Mismatched input

```
Line 3, Col 42: mismatched input ')' expecting ','
```

**Cause:** Syntax error in the MDL statement -- typically a missing comma, semicolon, or unmatched bracket.

**Solution:** Check the MDL syntax at the reported line and column. Common issues:
- Missing commas between attribute definitions
- Missing semicolons at the end of statements
- Unmatched parentheses or curly braces

### No viable alternative

```
Line 1, Col 0: no viable alternative at input 'CREAT'
```

**Cause:** Unrecognized keyword or misspelling.

**Solution:** Check the keyword spelling. MDL keywords are case-insensitive but must be valid. Run `mxcli syntax keywords` for the full keyword list.

## mxcli Execution Errors

### Module not found

```
Error: module 'MyModule' not found in project
```

**Cause:** The referenced module does not exist in the `.mpr` file.

**Solution:** Check the module name with `SHOW MODULES` and verify the spelling. Module names are case-sensitive.

### Entity not found

```
Error: entity 'MyModule.Customer' not found
```

**Cause:** The referenced entity does not exist in the specified module.

**Solution:** Check with `SHOW ENTITIES IN MyModule`. If the entity was just created, ensure the create statement executed successfully before referencing it.

### Reference validation failed

```
Error: unresolved reference 'MyModule.NonExistent' at line 5
```

**Cause:** A qualified name references an element that does not exist in the project. This error appears with `mxcli check script.mdl -p app.mpr --references`.

**Solution:** Verify the referenced element exists, or create it before the referencing statement.

## BSON Serialization Errors

### Wrong array prefix

**Symptom:** Studio Pro fails to load the project or shows garbled data.

**Cause:** Missing or incorrect integer prefix in BSON arrays. Mendix BSON arrays require a count/type prefix as the first element:

```json
{
  "Attributes": [3, { ... }, { ... }]
}
```

**Solution:** Ensure all arrays include the correct prefix value (typically `2` or `3`). Check existing BSON output for the correct prefix for each array property.

### Wrong reference format

**Symptom:** Studio Pro crashes or shows null reference errors.

**Cause:** Using `BY_ID_REFERENCE` (binary UUID) where `BY_NAME_REFERENCE` (qualified string) is expected, or vice versa.

**Solution:** Check the metamodel reflection data for the property's `kind` field:
- `BY_ID_REFERENCE` -> use binary UUID
- `BY_NAME_REFERENCE` -> use qualified name string (e.g., `"Module.Entity.Attribute"`)
