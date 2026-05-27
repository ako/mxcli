---
title: Improving LLM Assistance for MDL Code Generation
status: proposed
---

# Proposal: Improving LLM Assistance for MDL Code Generation

## Problem Statement

MDL (Mendix Definition Language) is a custom DSL that does not exist in LLM training data. When Claude Code or other LLMs attempt to write MDL, they rely entirely on:

- Grammar files (MDL.g4) - difficult to interpret programmatically
- Skill files - primary teaching mechanism but incomplete
- Example files - learning by pattern matching
- Error messages - learning what went wrong (reactive, not proactive)

This results in common mistakes like:
- Using `set` on undeclared variables
- Wrong syntax for entity type declarations (`type = empty` vs `as type`)
- Missing qualifications on association paths
- Incorrect enumeration comparisons (string literals vs qualified values)

## Goals

1. **Reduce iteration cycles** - LLM writes correct code on first attempt
2. **Self-correcting errors** - When errors occur, provide enough context to fix them
3. **Pattern consistency** - Encourage best practices and standard patterns
4. **Discoverability** - Make MDL capabilities easy to find and understand

## Proposed Improvements

### 1. Enhanced Error Messages with Examples

**Priority: High | Effort: Low**

Current error messages tell what's wrong but not how to fix it:

```
variable 'IsValid' is not declared. use declare IsValid: <type> before using set
```

Proposed format with inline example:

```
variable 'IsValid' is not declared.

Fix: add a declare statement before using set:

  declare $IsValid boolean = true;
  ...
  set $IsValid = false;
```

**Implementation:**

```go
// in cmd_microflows_builder.go
func (fb *flowBuilder) addErrorWithExample(message, example string) {
    fb.errors = append(fb.errors, fmt.Sprintf("%s\n\nExample:\n%s", message, example))
}

// Usage
fb.addErrorWithExample(
    fmt.Sprintf("variable '%s' is not declared", s.Target),
    fmt.Sprintf("  declare %s boolean = true;\n  set %s = false;", s.Target, s.Target),
)
```

**Error categories to enhance:**

| Error | Current Message | Proposed Addition |
|-------|-----------------|-------------------|
| Undeclared variable | "variable X not declared" | Show DECLARE + SET pattern |
| Entity type syntax | "selected type not allowed" | Show `declare $var as Module.Entity` |
| Association path | "error in expression" | Show `$var/Module.Association/attr` |
| Enum comparison | "type mismatch" | Show `Module.Enum.Value` syntax |

### 2. Focused Skills by Document Type / Use Case

**Priority: High | Effort: Medium**

Instead of one large reference, create smaller focused skills that can be loaded individually based on the task at hand. This keeps context focused and reduces token usage.

**Proposed skill organization:**

```
.claude/skills/mendix/
├── README.md                      # index of all skills
│
├── # by Document type (syntax reference)
├── mdl-entities.md                # entity, attributes, associations
├── mdl-enumerations.md            # enumeration syntax
├── mdl-microflows.md              # microflow syntax (exists: write-microflows.md)
├── mdl-pages.md                   # page and widget syntax
│
├── # by use case (patterns)
├── patterns-validation.md         # validation patterns (exists: validation-microflows.md)
├── patterns-crud.md               # create/read/update/delete patterns
├── patterns-data-processing.md    # Loops, aggregates, batch processing
├── patterns-integration.md        # rest, java actions, external calls
│
├── # Quick references (cheat sheets)
├── cheatsheet-variables.md        # Variable declaration quick ref
├── cheatsheet-expressions.md      # Operators, functions, xpath
├── cheatsheet-errors.md           # Common errors and fixes
│
└── # Existing skills
    ├── write-microflows.md
    ├── validation-microflows.md
    ├── write-oql-queries.md
    └── ...
```

**Skill loading strategy:**

1. **Task-based loading** - Load relevant skill based on user request:
   - "Create validation microflow" → load `patterns-validation.md`
   - "Add entity with attributes" → load `mdl-entities.md`

2. **Error-based loading** - When errors occur, suggest relevant skill:
   - Variable declaration error → reference `cheatsheet-variables.md`
   - XPath error → reference `cheatsheet-expressions.md`

3. **Keep skills small** - Target 100-200 lines per skill file

**Example: cheatsheet-variables.md (focused, ~50 lines)**

```markdown
# MDL Variable Cheatsheet

## Declaration Syntax

| type | Syntax | Example |
|------|--------|---------|
| string | `declare $name string = 'value';` | `declare $msg string = '';` |
| integer | `declare $name integer = 0;` | `declare $count integer = 0;` |
| boolean | `declare $name boolean = true;` | `declare $valid boolean = true;` |
| decimal | `declare $name decimal = 0.0;` | `declare $amount decimal = 0;` |
| datetime | `declare $name datetime = [%CurrentDateTime%];` | |
| entity | `declare $name as Module.Entity;` | `declare $cust as Sales.Customer;` |
| list | `declare $name list of Module.Entity = empty;` | |

## key rules

1. **Primitives**: `declare $var type = value;` (with initialization)
2. **entities**: `declare $var as Module.Entity;` (no initialization, use as)
3. **set requires declare**: Always declare before using set
4. **parameters are pre-declared**: No need to declare microflow parameters

## Common Mistakes

❌ `declare $product Module.Product = empty;` → Missing as
✅ `declare $product as Module.Product;`

❌ `set $isValid = true;` (without prior declare)
✅ `declare $isValid boolean = true;` then `set $isValid = false;`
```

**Example: patterns-crud.md (focused, ~150 lines)**

```markdown
# CRUD action Patterns

Patterns for create, read, update, delete operations on entities.

## ACT_Entity_Save (create or update)

```mdl
/**
 * Save action for Entity NewEdit page
 * Validates, commits, and closes the page
 */
CREATE MICROFLOW Module.ACT_Entity_Save (
  $Entity: Module.Entity
)
RETURNS Boolean
BEGIN
  -- Validate first
  $IsValid = CALL MICROFLOW Module.VAL_Entity_Save($Entity = $Entity);

  IF $IsValid THEN
    COMMIT $Entity;
    CLOSE PAGE;
  END IF;

  RETURN $IsValid;
END;
/
```

## ACT_Entity_Delete

```mdl
/**
 * Delete action with confirmation
 */
CREATE MICROFLOW Module.ACT_Entity_Delete (
  $Entity: Module.Entity
)
RETURNS Boolean
BEGIN
  DELETE $Entity;
  CLOSE PAGE;
  RETURN true;
END;
/
```

## when to use

- **ACT_Entity_Save**: Save button on NewEdit pages
- **ACT_Entity_Delete**: delete button with confirmation dialog
- **ACT_Entity_Cancel**: cancel button (just close page, no commit)
```

### 4. Check Command with Suggestions

**Priority: Medium | Effort: Medium**

Add `--suggest` flag to provide fix suggestions:

```bash
$ mxcli check script.mdl -p app.mpr --references --suggest

Checking: script.mdl
✓ Syntax OK (3 statements)

reference errors:
  statement 2: microflow 'Module.Test' has validation errors:
    - variable 'IsValid' is not declared

    Suggested fix (line 7):
    + declare $IsValid boolean = true;
      if $entity/Name = empty then
        set $IsValid = false;  -- line 9

✗ 1 error(s) found
```

**Implementation approach:**

1. Track source locations in AST nodes
2. Generate diff-style suggestions
3. Optionally apply fixes with `--fix` flag

### 5. DESCRIBE Enhancement for Learning

**Priority: Low | Effort: Low**

Add comments to DESCRIBE output explaining syntax:

```bash
$ mxcli -p app.mpr -c "describe microflow Module.Example --annotated"

-- Microflow signature: Name, parameters, return type
create microflow Module.Example (
  $Input: string           -- Parameter: $name: Type
)
returns boolean as $Result -- RETURNS Type AS $variableName
begin
  -- Variable declaration: DECLARE $name Type = value
  declare $Result boolean = true;

  -- Conditional: IF condition THEN ... END IF
  if $Input = empty then
    set $Result = false;   -- Assignment: SET $var = expression
  end if;

  return $Result;          -- Must end with RETURN
end;
/
```

### 6. Lint Rules as Teaching Tools

**Priority: Medium | Effort: Low**

Enhance lint rule output with educational content:

```python
# in lint rules
def check_undeclared_variable(node, context):
    if is_set_statement(node) and not is_declared(node.variable, context):
        return {
            "rule": "MDL020",
            "severity": "error",
            "message": f"Variable '{node.variable}' used in set but not declared",
            "learn_more": "https://docs.example.com/mdl/variables",
            "quick_fix": {
                "description": "add declare statement",
                "insert_before": node.line,
                "text": f"declare {node.variable} boolean = true; -- TODO: set correct type"
            }
        }
```

### 7. Interactive Examples in REPL

**Priority: Low | Effort: Medium**

Add `EXAMPLE` command to REPL:

```
mdl> EXAMPLE validation
-- Validation Microflow Pattern
create microflow Module.VAL_Entity_Action (
  $entity: Module.Entity
)
returns boolean as $IsValid
begin
  declare $IsValid boolean = true;

  if $entity/RequiredField = empty then
    set $IsValid = false;
    validation feedback $entity/RequiredField message 'Required';
  end if;

  return $IsValid;
end;
/

mdl> EXAMPLE loop
-- Loop Pattern
...
```

## Implementation Roadmap

### Phase 1: Quick Wins (1-2 days)
- [ ] Enhanced error messages with examples
- [ ] Create `cheatsheet-variables.md` skill (~50 lines)
- [ ] Create `cheatsheet-errors.md` skill (~50 lines)
- [ ] Update existing skills with more examples

### Phase 2: Focused Skills (3-5 days)
- [ ] Create `patterns-crud.md` skill
- [ ] Create `patterns-data-processing.md` skill
- [ ] Create `mdl-entities.md` syntax reference
- [ ] Update README.md with skill index and loading guidance

### Phase 3: Tooling (1 week)
- [ ] `--suggest` flag for check command
- [ ] Lint rules with educational output
- [ ] EXAMPLE command in REPL

### Phase 4: Advanced (future)
- [ ] Source location tracking in AST
- [ ] Auto-fix capability (`--fix` flag)
- [ ] Interactive tutorial mode

## Success Metrics

1. **First-attempt success rate** - % of LLM-generated MDL that passes check
2. **Iteration count** - Average attempts needed to get valid MDL
3. **Common error reduction** - Track top 10 errors, measure reduction
4. **User feedback** - Qualitative feedback on error message helpfulness

## Appendix: Common LLM Mistakes

Based on observed patterns:

| Mistake | Frequency | Root Cause |
|---------|-----------|------------|
| SET without DECLARE | High | No equivalent in most languages |
| Entity decl syntax | High | Unusual `as` keyword requirement |
| String enum comparison | Medium | Most languages use strings |
| Missing association qualification | Medium | XPath-style paths unfamiliar |
| Wrong DECLARE syntax (colon) | Medium | Confusion with TypeScript/Python |
| Missing RETURN | Low | Different from void functions |

## References

- [MDL Grammar](../../mdl/grammar/MDL.g4)
- [Existing Skills](../../.claude/skills/mendix/)
- [Example Files](../../mdl-examples/)
- [Write Microflows Skill](../../.claude/skills/mendix/write-microflows.md)
