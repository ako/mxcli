# Message definitions

Supporting reference for [json-structures-and-mappings](../SKILL.md).

A mapping's source can also be a **message definition** — 74 of the 327 mappings
in the demo corpus (22.6%), and the only non-JSON source MDL can create. Unlike
an XML schema (an imported `.xsd`) or a web service (a WSDL), it holds nothing
external: it is a **selection over the domain model**.

```sql
create message definition collection Sales.MD_Order
  folder 'Messages'
(
  definition OrderMessage for Sales.Order as 'Orders' (
    OrderId,
    Total as 'GrandTotal',
    Sales.OrderLine_Order/Sales.OrderLine as 'Lines' ( Sku, Quantity ),
    Sales.Order_Customer/Sales.Customer ( FirstName, Address example 'Kerstraat 5' )
  )
);
```

A **bare name is an attribute**; `Assoc/Module.Entity` is an **association** with
its own members — the same discriminator import and export mappings use. A
mapping then binds to `Module.Collection.Definition`, a three-part reference.

**Name the association's target entity.** It is not decoration: the stored
cardinality tracks the **direction of traversal**, not the association's type.
Reaching `Customer` from `Order` follows the foreign key and gives a single
object; reaching `Order` from `Customer` is the reverse and gives a list — the
same association, both ways. An association that connects the two entities in
neither direction is **refused**, because a wrong cardinality builds cleanly and
would silently expose a list as a single object.

**Inherited attributes are named like the entity's own.** mxcli resolves each to
the entity that declares it, which is what Mendix stores; qualifying one against
the entity that merely uses it is CE1613.

### Editing one without restating it

Real definitions nest deeply, so a whole-document rewrite is a poor tool for
"expose one more attribute". `ALTER` edits the stored document and leaves the
rest alone:

```sql
alter message definition Sales.MD_Order.OrderMessage add member Total;
alter message definition Sales.MD_Order.OrderMessage add member LastName in Customer;
alter message definition Sales.MD_Order.OrderMessage set member Total as 'GrandTotal';
alter message definition Sales.MD_Order.OrderMessage drop member Sku in Lines;

alter message definition collection Sales.MD_Order add definition Line for Sales.Line ( Sku );
alter message definition collection Sales.MD_Order rename definition Line to OrderLine;
alter message definition collection Sales.MD_Order drop definition if exists OrderLine;
```

`in <path>` reaches a nested member, written in **exposed names**. `SET` changes
only the exposed name — it is not a model rename, which is why the verb is not
`RENAME`.

Dropping or renaming a definition a mapping still references is refused, naming
the mappings.

### What mxcli does not guess

Studio Pro **pluralises** a repeating element's exposed name (`Order` →
`Orders`). mxcli defaults to the entity's own name and lets `as 'Orders'` say
otherwise — reproducing English inflection needs `-y → -ies` and an
already-plural detector, and a name the author writes beats one a heuristic
guesses. Everything else is derived from the domain model.

`show message definition collections [in Module]` lists them; `describe` emits
re-executable MDL.

### SOAP-sourced mappings are refused, not rewritten

A mapping can also be sourced from an **imported web service** — a WSDL binding
(which service, which operation, which root element). MDL cannot spell that, so
`create or replace|modify` over such a mapping is **refused** rather than
rebuilding it without the binding, which would leave it with no schema source at
all (**CE6896**, plus **CE0270**). `describe` marks the source instead of
emitting nothing, because the silent output parses and re-executing it is what
deletes the binding (ako/mxcli#365):

```sql
create or modify import mapping Legacy.IMM_Order
  -- SOURCE NOT REPRESENTABLE: imported web service Legacy.WS_Orders (service OrderService, operation GetOrder)
  -- re-executing this statement would drop it (CE6896); mxcli refuses the rewrite
{ ... }
```

Edit such a mapping in Studio Pro. Note that a consumed SOAP service does **not**
create XML schema documents — the WSDL's XSDs are held inline on the web-service
document — so `with xml schema` is a different path and does not help here.

### The `with` clause is resolved, not written through

A mapping's schema source — `with json structure M.X` or `with xml schema M.Y` —
is checked against the project by both `mxcli check -p` and `exec`, and a name
that resolves to nothing is refused with the documents that would have worked.
mxbuild otherwise reports it as **CE1613** "… no longer exists" at the end of a
build (ako/mxcli#259).

For JSON structures a typo used to be worse than a dangling reference: the schema
index is empty whenever the structure cannot be loaded **for any reason**, and an
empty index reads as "there is nothing to validate against" — so one typo in the
source name switched off every member check in the mapping.

Two things the check deliberately does not do:

- A structure the **same script** creates counts as existing. Create the
  structure, then map over it, is the normal shape.
- A project with **no** XML schemas disables the XML half rather than refusing
  every mapping. There is no `create xml schema` in MDL — an XML schema is only
  ever imported into the project by hand — so having none is ordinary, not
  evidence of a typo.

---
