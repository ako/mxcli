# Data Grid Column Filters — Detail

Loaded from [`overview-pages`](../SKILL.md) when a column filter is refused, lands
in the wrong place, or sits on an association. The type-to-widget table is in the
skill body.

**The filter goes inside the column's own braces.** A `filter { … }` block is
the GALLERY spelling of a different thing — the widget-wide filter bar, which a
data grid calls `controlbar`:

```sql
-- ✅ data grid: per-column filter, inside the column
datagrid dg (...) { column colName (attribute: Name) { textfilter f1 } }

-- ✅ gallery: the widget-wide filter bar, which the gallery calls `filter`
gallery g (...) { filter f { textfilter f1 } }

-- ❌ the gallery form on a data grid — MDL-WIDGET30
datagrid dg (...) { column colName (attribute: Name) filter f { textfilter f1 } }
```

That last line is worth reading twice: it is not a column with a filter block.
A widget is `type name (props) { body }`, so with the `filter` *outside* the
column's braces it parses as a column with **no body** followed by a separate
`filter` widget — which the grid has nowhere to put. It used to be dropped on
write with no diagnostic, so `DESCRIBE PAGE` showing a filterless column was the
only symptom; it is now refused at check and exec time.

**A column over an association is filtered by the associated objects.** The column
shows a value from the other side (`attribute: Order_Customer/Name`); the filter takes
the reference, the option list and what an option shows — all three, or it is refused:

```sql
column colCustomer (attribute: Order_Customer/Name, caption: 'Customer') {
  dropdownfilter fltCustomer (
    Association: Sales.Order_Customer,    -- the reference on the grid's entity
    datasource: database Sales.Customer,  -- the option list
    CaptionAttribute: Name                -- what each option shows
  )
}
```

A `datefilter` compares one date; `FilterType: between` makes the column a range.
