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
CREATE PAGE MyModule.Order_Edit
(
  Title: 'Edit Order',
  Layout: Atlas_Core.PopupLayout,
  Folder: 'Orders'
)
{
  CONTAINER main {}
}
```

## See Also

[CREATE MODULE](create-module.md), [DROP FOLDER](drop-folder.md), [MOVE](move.md)
