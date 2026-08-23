# Complete page examples

Supporting reference for [create-page](../SKILL.md).

## Complete Examples

### Customer Edit Page

```sql
create or replace page CRM.CustomerEdit
(
  params: { $Customer: CRM.Customer },
  title: 'Edit Customer',
  layout: Atlas_Core.PopupLayout
)
{
  -- Wrap the form's DataView in a layout grid. Label width and input-control
  -- width are expressed in Bootstrap grid columns and only render correctly
  -- inside a layoutgrid → row → column. A DataView with input fields placed
  -- directly on the page (no grid) is flagged by lint rule MPR010 / mxcli check.
  layoutgrid mainGrid {
    row row1 {
      column col1 (desktopwidth: autofill) {
        dataview dvCustomer (datasource: $Customer) {
          textbox txtName (label: 'Name', attribute: Name)
          textbox txtEmail (label: 'Email', attribute: Email)
          textbox txtPhone (label: 'Phone', attribute: Phone)
          checkbox cbActive (label: 'Active', attribute: IsActive)

          footer footer1 {
            actionbutton btnSave (caption: 'Save', action: save_changes, buttonstyle: primary)
            actionbutton btnCancel (caption: 'Cancel', action: cancel_changes)
          }
        }
      }
    }
  }
}
```

### Order Overview Page

```sql
create page Orders.OrderOverview
(
  title: 'Orders',
  layout: Atlas_Core.Atlas_Default
)
{
  layoutgrid mainGrid {
    row row1 {
      column col1 (desktopwidth: 12) {
        dynamictext heading (content: 'Order Overview', rendermode: H2)
      }
    }
    row row2 {
      column col2 (desktopwidth: 12) {
        datagrid dgOrders (datasource: database from Orders.Order sort by OrderDate desc) {
          column colNumber (attribute: OrderNumber, caption: 'Order #')
          column colDate (attribute: OrderDate, caption: 'Date')
          column colTotal (attribute: TotalAmount, caption: 'Total')
        }
      }
    }
  }
}
```

### Master-Detail Page

```sql
create page CRM.Customer_MasterDetail
(
  title: 'Customer Management',
  layout: Atlas_Core.Atlas_Default
)
{
  layoutgrid mainGrid {
    row row1 {
      -- Master list (left column)
      column colMaster (desktopwidth: 4) {
        dynamictext heading (content: 'Customers', rendermode: H3)
        gallery customerList (datasource: database from CRM.Customer sort by Name asc, selection: single) {
          template template1 {
            dynamictext name (content: '{1}', contentparams: [{1} = Name], rendermode: H4)
            dynamictext email (content: '{1}', contentparams: [{1} = Email])
          }
        }
      }

      -- Detail form (right column)
      column colDetail (desktopwidth: 8) {
        dataview customerDetail (datasource: selection customerList) {
          dynamictext detailHeading (content: 'Customer Details', rendermode: H3)
          textbox txtName (label: 'Name', attribute: Name)
          textbox txtEmail (label: 'Email', attribute: Email)
          textbox txtPhone (label: 'Phone', attribute: Phone)

          footer footer1 {
            actionbutton btnSave (caption: 'Save', action: save_changes, buttonstyle: primary)
            actionbutton btnCancel (caption: 'Cancel', action: cancel_changes)
          }
        }
      }
    }
  }
}
```
