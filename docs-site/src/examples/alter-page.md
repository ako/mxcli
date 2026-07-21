# Modifying Existing Pages

`ALTER PAGE` modifies widgets in place without recreating the entire page. This is useful for incremental changes, maintenance, and agent-driven iteration.

## Change a Button

```sql
ALTER PAGE CRM.Customer_Edit {
  SET (Caption = 'Save & Close', ButtonStyle = Success) ON btnSave
};
```

## Add a Field After an Existing One

```sql
ALTER PAGE CRM.Customer_Edit {
  INSERT AFTER txtEmail {
    TEXTBOX txtPhone (Label: 'Phone', Attribute: Phone)
  }
};
```

## Add a Widget Into a Container

Use `INSERT INTO` to append a widget as a container's last child — the only way to fill an empty container.

```sql
ALTER PAGE CRM.Customer_Overview {
  INSERT INTO ctnToolbar {
    ACTIONBUTTON btnNew (Caption: 'New Customer', Action: NOTHING, ButtonStyle: Primary)
  }
};
```

## Remove Widgets

```sql
ALTER PAGE CRM.Customer_Edit {
  DROP WIDGET txtLegacyField, lblOldNote
};
```

## Replace an Entire Footer

```sql
ALTER PAGE CRM.Customer_Edit {
  REPLACE footer1 WITH {
    FOOTER newFooter {
      ACTIONBUTTON btnSave (Caption: 'Save', Action: SAVE_CHANGES, ButtonStyle: Success)
      ACTIONBUTTON btnDelete (Caption: 'Delete', Action: DELETE, ButtonStyle: Danger)
      ACTIONBUTTON btnCancel (Caption: 'Cancel', Action: CANCEL_CHANGES)
    }
  }
};
```

## Multiple Changes at Once

```sql
ALTER PAGE CRM.Customer_Edit {
  SET Title = 'Edit Customer Details';
  SET Label = 'Email Address' ON txtEmail;
  INSERT AFTER txtPhone {
    TEXTBOX txtWebsite (Label: 'Website', Attribute: Website)
  };
  DROP WIDGET lblInternalRef
};
```

## Add a Page Variable

```sql
ALTER PAGE CRM.ProductOverview {
  ADD Variables $showStockColumn: Boolean = 'if (3 < 4) then true else false'
};
```

## Switch Page Layout

Change a page's layout without losing any widgets:

```sql
-- Switch from TopBar to Default layout (auto-maps by placeholder name)
ALTER PAGE CRM.Customer_Edit {
  SET Layout = Atlas_Core.Atlas_Default
};
```

When the new layout has different placeholder names, use `MAP`:

```sql
ALTER PAGE CRM.Customer_Edit {
  SET Layout = Atlas_Core.Atlas_SideBar MAP (Main AS Content, Extra AS Sidebar)
};
```

## Modify DataGrid Columns

Target columns using dotted notation `gridName.columnName`:

```sql
-- Add a column
ALTER PAGE CRM.Customer_List {
  INSERT AFTER dgCustomers.Email {
    COLUMN Phone (Attribute: Phone, Caption: 'Phone')
  }
};

-- Remove a column
ALTER PAGE CRM.Customer_List {
  DROP WIDGET dgCustomers.OldNotes
};

-- Rename a column header
ALTER PAGE CRM.Customer_List {
  SET Caption = 'E-mail Address' ON dgCustomers.Email
};
```

Use `DESCRIBE PAGE CRM.Customer_List` to discover column names.

## Works on Snippets Too

```sql
ALTER SNIPPET CRM.NavigationMenu {
  SET Caption = 'Dashboard' ON btnHome;
  INSERT AFTER btnHome {
    ACTIONBUTTON btnReports (
      Caption: 'Reports',
      Action: SHOW_PAGE CRM.Reports_Overview
    )
  }
};
```
