# CREATE FOLDER

## Synopsis

    CREATE FOLDER module_name/folder_path

## Description

Creates a folder within a module for organizing documents such as pages, microflows, and snippets. Nested folders use the `/` separator. If intermediate folders in the path do not exist, they are created automatically.

Note: Folders can also be created implicitly by specifying a `FOLDER` clause when creating a microflow or a `Folder` property when creating a page.

## Parameters

**module_name**
: The name of the module in which to create the folder.

**folder_path**
: The path of the folder to create within the module. Use `/` to create nested folders (e.g., `Orders/Processing`).

## Examples

### Create a simple folder

```sql
CREATE FOLDER MyModule/Pages;
```

### Create a nested folder

```sql
CREATE FOLDER MyModule/Orders/Processing;
```

### Use a folder when creating a microflow

```sql
CREATE MICROFLOW MyModule.ACT_ProcessOrder
FOLDER 'Orders/Processing'
BEGIN
  RETURN true;
END;
```

### Use a folder when creating a page

```sql
CREATE PAGE MyModule.Order_Edit FOLDER 'Orders'
(
  Title: 'Edit Order',
  Layout: Atlas_Core.PopupLayout
)
{
  CONTAINER main {}
};
```

### A folder whose name contains `/`

Studio Pro allows `/` inside a folder name, so in a `FOLDER '…'` path a slash
that belongs to the name is written `\/` (and a backslash `\\`). This is one
folder, `Private - String en/de-cryption`, holding `Apis`:

```sql
CREATE CONSTANT Encryption.EncryptionKey FOLDER 'Private - String en\/de-cryption/Apis' (
  Type: String,
  DefaultValue: ''
);
```

`DESCRIBE` writes the escape for you, so its output files the document back
where it was. A backslash before any other character is an ordinary character.

## See Also

[CREATE MODULE](create-module.md), [DROP FOLDER](drop-folder.md), [MOVE](move.md)
