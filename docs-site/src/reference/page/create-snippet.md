# CREATE SNIPPET

## Synopsis

```sql
CREATE [ OR REPLACE ] SNIPPET module.Name
(
    [ Params: { $param : Module.Entity | Type [, ...] } ]
    [, Folder: 'path' ]
)
{
    widget_tree
}
```

## Description

Creates a reusable widget fragment (snippet) in the specified module. Snippets contain a widget tree that can be embedded in pages or other snippets using the `SNIPPETCALL` widget.

If `OR REPLACE` is specified and a snippet with the same qualified name already exists, it is replaced.

Snippets support the same widget types and syntax as pages. The key difference is that snippets do not have a `Title` or `Layout` -- they are fragments meant to be included within a page's layout.

### Snippet Parameters

Snippets can optionally declare parameters. When a snippet has parameters, the `SNIPPETCALL` widget that embeds it must provide the corresponding values.

### Folder Placement

The optional `Folder` property places the snippet in a subfolder within the module.

## Parameters

`module.Name`
:   The qualified name of the snippet (`Module.SnippetName`). The module must already exist.

`Params: { ... }`
:   Optional snippet parameters. Each parameter has a `$`-prefixed name and an
    **entity** type (`Module.Entity`).

    A snippet parameter cannot be a primitive. Mendix rejects one with
    **CE0046** *"Invalid data type 'String'."* — a *page* parameter may be a
    primitive, a snippet parameter may not — so `mxcli check` refuses it as
    **MDL087** rather than letting it reach a build. To parameterise a snippet
    on a value, keep the primitive on the calling page's parameters, or pass an
    object and read the member inside the snippet.

`Folder: 'path'`
:   Optional folder path within the module.

## Examples

Simple snippet with a header:

```sql
CREATE SNIPPET MyModule.CustomerHeader
(
    Params: { $Customer: MyModule.Customer }
)
{
    CONTAINER cntHeader (Class: 'card-header') {
        DYNAMICTEXT txtName (Attribute: Name)
        DYNAMICTEXT txtEmail (Attribute: Email)
    }
};
```

Snippet with form fields:

```sql
CREATE SNIPPET MyModule.AddressFields
(
    Params: { $Address: MyModule.Address }
)
{
    TEXTBOX txtStreet (Label: 'Street', Attribute: Street)
    TEXTBOX txtCity (Label: 'City', Attribute: City)
    TEXTBOX txtZip (Label: 'Zip Code', Attribute: ZipCode)
    TEXTBOX txtCountry (Label: 'Country', Attribute: Country)
};
```

Snippet without parameters:

```sql
CREATE SNIPPET MyModule.AppFooter
{
    CONTAINER cntFooter (Class: 'app-footer') {
        DYNAMICTEXT txtVersion (Attribute: Version)
    }
};
```

Embedding a snippet in a page:

```sql
CREATE PAGE MyModule.Customer_Edit
(
    Params: { $Customer: MyModule.Customer },
    Title: 'Edit Customer',
    Layout: Atlas_Core.PopupLayout
)
{
    DATAVIEW dvCustomer (DataSource: $Customer) {
        SNIPPETCALL snpAddress (Snippet: MyModule.AddressFields)
        FOOTER footer1 {
            ACTIONBUTTON btnSave (Caption: 'Save', Action: SAVE_CHANGES, ButtonStyle: Primary)
        }
    }
};
```

## See Also

[CREATE PAGE](create-page.md), [ALTER PAGE](alter-page.md), [DROP PAGE](drop-page.md)
