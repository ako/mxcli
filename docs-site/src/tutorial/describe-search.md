# DESCRIBE, SEARCH

While `SHOW` commands list elements by name, `DESCRIBE` gives you the full definition of a single element. `SEARCH` lets you find elements by keyword when you do not know the exact name.

## DESCRIBE ENTITY

To see the complete definition of an entity, including its attributes, types, and constraints:

```sql
DESCRIBE ENTITY MyFirstModule.Customer;
```

Example output:

```sql
CREATE PERSISTENT ENTITY MyFirstModule.Customer (
  Name: String(200),
  Email: String(200),
  Phone: String(50),
  DateOfBirth: DateTime,
  IsActive: Boolean DEFAULT false
);
```

The output is valid MDL -- you could copy it, modify it, and execute it as a `CREATE OR MODIFY` statement. This round-trip capability is one of the key design features of MDL.

DESCRIBE also shows associations and access rules when they exist:

```sql
DESCRIBE ENTITY MyFirstModule.Order;
```

```sql
CREATE PERSISTENT ENTITY MyFirstModule.Order (
  OrderNumber: AutoNumber,
  OrderDate: DateTime,
  TotalAmount: Decimal,
  Status: MyFirstModule.OrderStatus
);

CREATE ASSOCIATION MyFirstModule.Order_Customer
  FROM MyFirstModule.Order TO MyFirstModule.Customer
  TYPE Reference
  OWNER Default
  DELETE_BEHAVIOR DELETE_BUT_KEEP_REFERENCES;
```

## DESCRIBE MICROFLOW

To see the full logic of a microflow:

```sql
DESCRIBE MICROFLOW MyFirstModule.ACT_Customer_Save;
```

Example output:

```sql
CREATE MICROFLOW MyFirstModule.ACT_Customer_Save
  ($Customer: MyFirstModule.Customer)
  RETURNS Boolean
BEGIN
  IF $Customer/Name = empty THEN
    VALIDATION FEEDBACK $Customer/Name MESSAGE 'Name is required';
    RETURN false;
  END IF;

  COMMIT $Customer;
  RETURN true;
END;
```

This shows the parameters, return type, and the complete flow logic. For complex microflows, this can be quite long -- but it gives you the full picture in one command.

## DESCRIBE PAGE

Pages are shown as their widget tree:

```sql
DESCRIBE PAGE MyFirstModule.Customer_Edit;
```

Example output:

```sql
CREATE PAGE MyFirstModule.Customer_Edit
(
  Params: { $Customer: MyFirstModule.Customer },
  Title: 'Edit Customer',
  Layout: Atlas_Core.PopupLayout
)
{
  DATAVIEW dvCustomer (DataSource: $Customer) {
    TEXTBOX txtName (Label: 'Name', Attribute: Name)
    TEXTBOX txtEmail (Label: 'Email', Attribute: Email)
    TEXTBOX txtPhone (Label: 'Phone', Attribute: Phone)

    FOOTER footer1 {
      ACTIONBUTTON btnSave (Caption: 'Save', Action: SAVE_CHANGES, ButtonStyle: Primary)
      ACTIONBUTTON btnCancel (Caption: 'Cancel', Action: CANCEL_CHANGES)
    }
  }
};
```

## DESCRIBE ENUMERATION

```sql
DESCRIBE ENUMERATION MyFirstModule.OrderStatus;
```

```sql
CREATE ENUMERATION MyFirstModule.OrderStatus (
  Draft 'Draft',
  Submitted 'Submitted',
  Approved 'Approved',
  Rejected 'Rejected',
  Completed 'Completed'
);
```

Each value is followed by its caption (the display label shown to end users).

## DESCRIBE ASSOCIATION

```sql
DESCRIBE ASSOCIATION MyFirstModule.Order_Customer;
```

```sql
CREATE ASSOCIATION MyFirstModule.Order_Customer
  FROM MyFirstModule.Order TO MyFirstModule.Customer
  TYPE Reference
  OWNER Default
  DELETE_BEHAVIOR DELETE_BUT_KEEP_REFERENCES;
```

## DESCRIBE MODULE

To see the full contents of a module (all entities, microflows, pages, etc.):

```sql
DESCRIBE MODULE MyFirstModule;
```

This is especially useful from the CLI:
```bash
mxcli describe -p app.mpr module MyFirstModule
```

## Other DESCRIBE targets

The same pattern works for other element types:

```sql
DESCRIBE MODULE MyFirstModule;
DESCRIBE WORKFLOW MyFirstModule.ApprovalFlow;
DESCRIBE NANOFLOW MyFirstModule.NAV_GoToDetail;
DESCRIBE SNIPPET MyFirstModule.CustomerCard;
DESCRIBE CONSTANT MyFirstModule.ApiBaseUrl;
DESCRIBE JSON STRUCTURE MyFirstModule.CustomerResponse;
DESCRIBE IMPORT MAPPING MyFirstModule.IMM_Customer;
DESCRIBE EXPORT MAPPING MyFirstModule.EMM_Customer;
DESCRIBE REST CLIENT MyFirstModule.PetStoreAPI;
DESCRIBE IMAGE COLLECTION MyFirstModule.Icons;
DESCRIBE NAVIGATION;
DESCRIBE SETTINGS;
```

## Full-text search with SEARCH

When you do not know the exact name of an element, use `SEARCH` to find it by keyword. This searches across all strings in the project -- entity names, attribute names, microflow logic, page captions, messages, documentation, and more.

```sql
SEARCH 'validation';
```

Example output:

```
MyFirstModule.ACT_Customer_Save (Microflow)
  "VALIDATION FEEDBACK $Customer/Name MESSAGE 'Name is required'"

MyFirstModule.ACT_Order_Validate (Microflow)
  "VALIDATION FEEDBACK $Order/OrderDate MESSAGE 'Order date cannot be in the past'"

MyFirstModule.Customer_Edit (Page)
  "Validation errors will appear here"
```

Search is case-insensitive and matches partial words.

### Search from the command line

The CLI provides a dedicated `search` subcommand with formatting options:

```bash
# Search with default output
mxcli search -p app.mpr "validation"

# Show only element names (no context)
mxcli search -p app.mpr "validation" --format names

# JSON output for programmatic use
mxcli search -p app.mpr "validation" --format json
```

The `--format names` option is useful for piping into other commands:

```bash
# Find all microflows mentioning "email" and describe each one
mxcli search -p app.mpr "email" --format names | while read name; do
  mxcli -p app.mpr -c "DESCRIBE MICROFLOW $name"
done
```

## Combining SHOW, DESCRIBE, and SEARCH

A typical exploration workflow looks like this:

1. **Start broad** with SHOW to see what exists:
   ```sql
   SHOW MODULES;
   SHOW ENTITIES IN Sales;
   ```

2. **Zoom in** with DESCRIBE on interesting elements:
   ```sql
   DESCRIBE ENTITY Sales.Order;
   DESCRIBE MICROFLOW Sales.ACT_Order_Process;
   ```

3. **Search** when you need to find something specific:
   ```sql
   SEARCH 'discount';
   ```

This workflow mirrors how you would explore a project in Mendix Studio Pro -- browsing the project explorer, opening documents, and using Find to locate things.

Next, learn how to get a compact overview of the entire project with [SHOW STRUCTURE](show-structure.md).
