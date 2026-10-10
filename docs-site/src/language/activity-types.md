# Activity Types

This page documents every activity type available in MDL microflows. Activities are the individual steps that make up a microflow's logic.

## Object Operations

### CREATE

Creates a new object of the specified entity type and assigns attribute values:

```sql
$Order = CREATE Sales.Order (
  OrderDate = [%CurrentDateTime%],
  Status = 'Draft',
  TotalAmount = 0
);
```

Attribute assignments are comma-separated `Name = value` pairs. The result is assigned to a variable.

### CHANGE

Modifies attributes of an existing object:

```sql
CHANGE $Order (
  Status = 'Confirmed',
  TotalAmount = $CalculatedTotal
);
```

Multiple attributes can be changed in a single `CHANGE` statement.

### COMMIT

Persists an object (or its changes) to the database:

```sql
COMMIT $Order;
```

Options:
- `WITHOUT EVENTS` -- skips the event handlers (before/after commit microflows), which otherwise run
- `REFRESH` -- refreshes the object in the client after committing

```sql
COMMIT $Order;
COMMIT $Order REFRESH;
COMMIT $Order REFRESH;
```

### DELETE

Deletes an object from the database:

```sql
DELETE $Order;
```

### ROLLBACK

Reverts uncommitted changes to an object, restoring it to its last committed state:

```sql
ROLLBACK $Order;
ROLLBACK $Order REFRESH;
```

The `REFRESH` option refreshes the object in the client.

## Retrieval

### RETRIEVE from Database (XPath)

Retrieves objects from the database using an optional XPath-style WHERE clause:

```sql
-- Retrieve a single object
RETRIEVE $Customer FROM Sales.Customer
  WHERE Email = $InputEmail
  FIRST;

-- Retrieve a list of objects
RETRIEVE $ActiveOrders FROM Sales.Order
  WHERE Status = 'Active';

-- Retrieve all objects of an entity
RETRIEVE $AllProducts FROM Sales.Product;

-- Retrieve with multiple conditions
RETRIEVE $RecentOrders FROM Sales.Order
  WHERE Status = 'Active'
  AND OrderDate > [%BeginOfCurrentDay%]
  LIMIT 50;
```

With `FIRST` the result is a single entity object (Mendix's "First object" range). Otherwise the result is a list, including with `LIMIT`/`OFFSET`. A bare `LIMIT 1` is a list of one (a script file without the header reads it differently; see [Language Versions and Migration](versions.md)). `describe` prints the object range as `FIRST`.

### RETRIEVE by Association

Retrieves objects by following an association from an existing object:

```sql
-- Retrieve related objects via association
RETRIEVE $Lines FROM $Order/Sales.OrderLine_Order;

-- Retrieve a single associated object
RETRIEVE $Customer FROM $Order/Sales.Order_Customer;
```

The association path uses the format `$Variable/Module.AssociationName`.

### RETRIEVE from Variable List

Retrieves from an in-memory list variable:

```sql
RETRIEVE $Match FROM $CustomerList
  WHERE Email = $SearchEmail
  LIMIT 1;
```

## Call Activities

### CALL MICROFLOW

Calls another microflow, passing parameters and optionally receiving a return value:

```sql
-- Call with return value
$Total = CALL MICROFLOW Sales.SUB_CalculateTotal (
  OrderLines = $Lines
);

-- Call without return value
CALL MICROFLOW Sales.SUB_SendNotification (
  Customer = $Customer,
  Message = 'Order confirmed'
);

-- Call with no parameters
$Config = CALL MICROFLOW Admin.GetSystemConfig ();
```

### CALL NANOFLOW

Calls a nanoflow from within a microflow:

```sql
$Result = CALL NANOFLOW MyModule.ValidateInput (
  InputValue = $Value
);
```

### CALL JAVA ACTION

Calls a Java action:

```sql
$Hash = CALL JAVA ACTION MyModule.HashPassword (
  Password = $RawPassword
);
```

Java action parameters follow the same `Name = value` syntax.

## UI Activities

### SHOW PAGE

Opens a page, passing parameters:

```sql
SHOW PAGE Sales.Order_Edit (Order = $Order);
```

The parameter syntax is `PageParam = $MicroflowVar`, with no `$` on the page's parameter name. Multiple parameters are comma-separated:

```sql
SHOW PAGE Sales.OrderDetail (
  Order = $Order,
  Customer = $Customer
);
```

The older spellings `($Order = $Order)` and `(Order: $Order)` still parse but are
deprecated (MDL-DEPR006, MDL-DEPR007); `mxcli fmt --upgrade` rewrites them.

### CLOSE PAGE

Closes the current page, or the given number of pages:

```sql
CLOSE PAGE;      -- the current page
CLOSE PAGE 2;    -- the current page and the one that opened it
```

The count is a whole number of at least 1. Mendix stores it as
`NumberOfPagesToClose`, and `describe` prints it when it is more than one.

## Validation

### VALIDATION FEEDBACK

Displays a validation error message on a specific attribute of an object:

```sql
VALIDATION FEEDBACK $Customer/Email MESSAGE 'Email address is required';
VALIDATION FEEDBACK $Order/TotalAmount MESSAGE 'Total must be greater than zero';
```

The syntax is `VALIDATION FEEDBACK $Variable/AttributeName MESSAGE 'message text'`.

## Logging

### LOG

Writes a message to the Mendix runtime log:

```sql
LOG INFO 'Order created successfully';
LOG WARNING 'Customer has no email address';
LOG ERROR 'Failed to process payment';
```

Log levels: `INFO`, `WARNING`, `ERROR`.

Optionally specify a log node name:

```sql
LOG INFO NODE 'OrderProcessing' 'Order created: ' + $Order/OrderNumber;
LOG ERROR NODE 'PaymentGateway' 'Payment failed for order ' + $Order/OrderNumber;
```

## Email

### SEND EMAIL

Sends mail over SMTP with the built-in **Send Email** activity (Studio Pro 11.13+,
beta) — no Email Connector module needed. Microflows only.

```sql
SEND EMAIL (
  From: @Shop.MailSender,
  To: $Order/CustomerEmail,
  Cc: 'archive@example.com',
  Subject: 'Order {1} confirmed' WITH ({1} = $Order/OrderNumber),
  Body: TEMPLATE 'Hi {1}, your order ships soon.' WITH ({1} = $Order/CustomerName),
  HtmlBody: TEMPLATE '<p>Your order ships soon.</p>',
  Headers: ('X-Correlation-Id': 'order-confirmation'),
  Attachment: $Invoice,
  Host: @Shop.SmtpHost,
  Port: @Shop.SmtpPort,
  SecurityType: SSL,
  CheckServerIdentity: true,
  ConnectionTimeout: 30000,
  Authentication: BASIC (Username: @Shop.SmtpUser, Password: @Shop.SmtpPassword),
) ON ERROR WITHOUT ROLLBACK BEGIN
  LOG ERROR NODE 'Mail' 'Sending failed';
END ERROR;
```

The activity's settings are one property list, as for `CALL REST SERVICE`
([Activity Settings](basics.md)); the order of the keys does not matter. `From`,
`Host`, `Port` and at least one of `To`, `Cc` and `Bcc` are required — mxbuild
reports a missing one as CE0166.

- **Addresses, host, port, user and password** are expressions. All are String
  except `Port`, which is Integer or Long. Prefer constants for the connection
  settings, and never a literal password.
- **Subject** is a text template, as in `LOG`; **Body** and **HtmlBody** take
  `template '…'`, as a REST body does. Placeholders `{1}`, `{2}` … are filled by
  String expressions in `WITH`. Any other expression is stored as the template `'{1}'`.
- **Headers** are plain strings: the name holds letters, digits and hyphens.
- **Attachment** is a `System.FileDocument` variable, a list of them, or a
  specialization.
- **Defaults**, which `DESCRIBE` leaves out: `SecurityType: tls`,
  `ConnectionTimeout: 20000` (milliseconds), no `CheckServerIdentity` (it applies
  to SSL only), no authentication.

Studio Pro's *Test Email* tab is an editor tool and is not part of MDL; rewriting
the microflow stores it empty.

## Database Query Execution

### EXECUTE DATABASE QUERY

Executes a Database Connector query defined in the project:

```sql
-- Basic execution (3-part qualified name: Module.Connection.Query)
$Result = EXECUTE DATABASE QUERY MyModule.MyConn.GetCustomers;

-- Dynamic SQL query
$Result = EXECUTE DATABASE QUERY MyModule.MyConn.SearchQuery
  DYNAMIC 'SELECT * FROM customers WHERE name LIKE ?';

-- With parameters
$Result = EXECUTE DATABASE QUERY MyModule.MyConn.GetByEmail (EmailParam = $Email);

-- With runtime connection override
$Result = EXECUTE DATABASE QUERY MyModule.MyConn.GetData
  CONNECTION $RuntimeConnString;
```

The query name follows a three-part naming convention: `Module.ConnectionName.QueryName`.

> **Note:** Error handling (`ON ERROR`) is not supported on `EXECUTE DATABASE QUERY` activities.

## Summary Table

| Activity | Syntax | Returns |
|----------|--------|---------|
| Create object | `$Var = CREATE Module.Entity (Attr = val) [COMMIT [WITHOUT EVENTS]] [REFRESH];` | Entity object |
| Change object | `CHANGE $Var (Attr = val) [COMMIT [WITHOUT EVENTS]] [REFRESH];` | -- |
| Commit | `COMMIT $Var [WITHOUT EVENTS] [REFRESH];` | Omitted = with events |
| Delete | `DELETE $Var [REFRESH];` | -- |
| Rollback | `ROLLBACK $Var [REFRESH];` | -- |
| Retrieve (DB) | `RETRIEVE $Var FROM Module.Entity [WHERE ...] [LIMIT n];` | Entity or list |
| Retrieve (assoc) | `RETRIEVE $Var FROM $Obj/Module.Assoc;` | Entity or list |
| Call microflow | `$Var = CALL MICROFLOW Module.Name (Param = $val);` | Any type |
| Call nanoflow | `$Var = CALL NANOFLOW Module.Name (Param = $val);` | Any type |
| Call Java action | `$Var = CALL JAVA ACTION Module.Name (Param = val);` | Any type |
| Show page | `SHOW PAGE Module.Page (Param = $val);` | -- |
| Close page | `CLOSE PAGE [n];` | -- |
| Validation | `VALIDATION FEEDBACK $Var/Attr MESSAGE 'msg';` | -- |
| Log | `LOG INFO\|WARNING\|ERROR [NODE 'name'] 'msg';` | -- |
| DB query | `$Var = EXECUTE DATABASE QUERY Module.Conn.Query;` | Result set |
| Send email | `SEND EMAIL (From: …, To: …, Subject: '…', Host: …, Port: …);` (11.13+) | -- |
| Assignment | `SET $Var = expression;` | -- |
| Change list | `ADD $Item TO $List;` / `REMOVE $Item FROM $List;` / `CLEAR $List;` / `SET $List = $Other;` | -- |
