# Case Study: Recreating MxGraphStudioDemo in MDL

## Overview

The **MxGraphStudioDemo** project (Mendix 11.6.3) is a small app that connects to a Graph Studio OData endpoint to display and edit customer/address data and visualize a hierarchical Bill of Materials (BOM) tree. This document analyzes what MDL can express today and provides scripts to recreate the supported portions.

## Project Structure

| Module | Role | Source |
|--------|------|--------|
| **OdataPlm** | App module — external entities from OData, overview/edit pages, BOM tree | Custom |
| **Main** | App module — home page, REST integration prototype, constants | Custom |
| Administration | Marketplace module — user management | v4.3.2 |
| Atlas_Core | Marketplace module — layouts and building blocks | v4.1.3 |
| Atlas_Web_Content | Marketplace module — page templates | v4.1.0 |
| DataWidgets | Marketplace module — DataGrid2, filters | v3.5.0 |
| FeedbackModule | Marketplace module — feedback widget | v4.0.3 |

## Architecture

```
Graph Studio odata api
    |
    v
[OdataPlm.MxPlmOdataApiClient] -- consumed OData service
    |
    v
external entities: Customer, Address, Bom, Component, Sub_Component
    |
    v
pages: Customer_Overview, Address_Overview, Customer_NewEdit, Address_NewEdit, BomTree
    |
    v
navigation: Responsive -> Main.Home_Web -> buttons to OdataPlm pages
```

### Associations

- `OdataPlm.Address_2`: Customer -> Address (Reference)
- `OdataPlm.Components`: Bom -> Component (ReferenceSet)
- `OdataPlm.Sub_Components`: Component -> Sub_Component (ReferenceSet)

## MDL Coverage Analysis

### Fully Supported

| Feature | MDL Syntax | Status |
|---------|-----------|--------|
| External entities | `create external entity` | Supported |
| Persistent entities | `create persistent entity` | Supported |
| Non-persistent entities | `create non-persistent entity` | Supported |
| Enumerations | `create enumeration` | Supported |
| Overview pages (DataGrid2 + filters) | `create page ... { datagrid ... }` | Supported |
| Edit pages (DataView + TextBox) | `create page ... { dataview ... }` | Supported |
| Snippets | `create snippet` | Supported |
| Navigation lists | `navigationlist` | Supported |
| Layout grids | `layoutgrid / row / column` | Supported |
| Action buttons | `actionbutton` | Supported |
| Text/Number filters | `textfilter / numberfilter` | Supported |
| Dynamic text / headings | `dynamictext` | Supported |
| ComboBox | `combobox` | Supported |
| Navigation profiles | `create or replace navigation` | Supported |
| Constants | `create constant` | Supported |
| Page parameters | `params: { $Var: entity }` | Supported |
| Show page actions | `action: show_page Module.Page` | Supported |
| Save/Cancel actions | `action: save_changes / cancel_changes` | Supported |
| Consumed OData clients | `create odata client` | Supported |
| External entities with OData source | `create external entity ... from odata client` | Supported |

### Not Supported

| Feature | What's Missing | Workaround |
|---------|---------------|------------|
| **TreeNode widget** | Pluggable widget not in MDL grammar or SDK. `describe page` outputs `TREENODE treeNode1` but cannot configure it. | No workaround — requires Studio Pro to configure. Can create the page structure and add TreeNode manually. |
| **RestOperationCallAction** | Microflow activity not implemented in describe/create. Shows as `-- Unsupported action type`. | Must create REST call microflows in Studio Pro. |

### Partially Supported

| Feature | Notes |
|---------|-------|
| **BomTree page** | The page layout (LayoutGrid, DynamicText heading) is fully supported. Only the `TREENODE` widget inside it cannot be configured — MDL outputs just the widget name with no properties. |
| **Main.GetCustomers microflow** | The `retrieve` and `return` are supported, but the `RestOperationCallAction` at the start is emitted as a comment. |

## MDL Scripts to Recreate the App

### Step 1: Create Modules

```sql
-- The OdataPlm and Main modules must exist first.
-- Marketplace modules (Atlas_Core, Administration, etc.) are assumed to be
-- installed via the Mendix Marketplace in Studio Pro.
create module OdataPlm;
create module Main;
```

### Step 2: Create Constants (needed by OData client)

```sql
create constant Main.MxPlmGraphClient_graphmart as string = 'http%3A%2F%2Fcambridgesemantics.com%2F...';
create constant Main.MxPlmGraphClient_graphstudio_api as string = 'https://graphstudio.mendixdemo.com/sp...';
create constant Main.MxPlmGraphClient_password as string = 'Welcome1!';
create constant Main.MxPlmGraphClient_username as string = 'Administrator';
create constant OdataPlm.MxPlmOdataApiClient_Location as string = 'https://graphstudio.mendixdemo.com/da...';
```

### Step 3: Create the Consumed OData Client

```sql
create odata client OdataPlm.MxPlmOdataApiClient (
  ODataVersion: OData4,
  MetadataUrl: 'https://graphstudio.mendixdemo.com/dataondemand/Mx-PLM-example/MxPlmExample/$metadata',
  timeout: 300,
  ServiceUrl: '@OdataPlm.MxPlmOdataApiClient_Location',
  UseAuthentication: Yes,
  HttpUsername: '@Main.MxPlmGraphClient_username',
  HttpPassword: '@Main.MxPlmGraphClient_password'
);
/
```

### Step 4: External Entities (OdataPlm)

```sql
create external entity OdataPlm.Customer
from odata client OdataPlm.MxPlmOdataApiClient
(
  EntitySet: 'Customer',
  RemoteName: 'Customer',
  Countable: Yes,
  Creatable: No,
  Deletable: No,
  Updatable: No
)
(
  customer_key: string,
  Industry: string,
  address_key: string,
  Name: string,
  _Id: string,
  Phone: string,
  Email: string,
  Contact_Person: string
);
/

create external entity OdataPlm.Address
from odata client OdataPlm.MxPlmOdataApiClient
(
  EntitySet: 'Address',
  RemoteName: 'Address',
  Countable: Yes,
  Creatable: No,
  Deletable: No,
  Updatable: No
)
(
  address_key: string,
  Country: string,
  State: string,
  Street: string,
  Post_Code: string,
  City: string,
  Zip_Code: integer
);
/

create external entity OdataPlm.Bom
from odata client OdataPlm.MxPlmOdataApiClient
(
  EntitySet: 'Bom',
  RemoteName: 'Bom',
  Countable: Yes,
  Creatable: No,
  Deletable: No,
  Updatable: No
)
(
  bom_key: string,
  Product_Version: decimal,
  Product_Id: string,
  _Id: string,
  Name: string
);
/

create external entity OdataPlm.Component
from odata client OdataPlm.MxPlmOdataApiClient
(
  EntitySet: 'Component',
  RemoteName: 'Component',
  Countable: Yes,
  Creatable: No,
  Deletable: No,
  Updatable: No
)
(
  component_key: string,
  Name: string,
  Quantity_Required: long,
  level: long,
  _Id: string,
  Unit_Of_Measure: string
);
/

create external entity OdataPlm.Sub_Component
from odata client OdataPlm.MxPlmOdataApiClient
(
  EntitySet: 'Sub_Component',
  RemoteName: 'Sub_Component',
  Countable: Yes,
  Creatable: No,
  Deletable: No,
  Updatable: No
)
(
  sub_component_key: string,
  Unit_Of_Measure: string,
  Quantity_Required: long,
  Component_Name: string,
  level: long,
  Component_Id: string
);
/
```

### Step 5: Test Entity and Enumeration (OdataPlm)

```sql
create enumeration OdataPlm.Test (
  wewe 'wewe'
);
/

create persistent entity OdataPlm.TestAbc (
  test: enumeration(OdataPlm.Test)
);
/
```

### Step 6: Non-Persistent Entities (Main)

```sql
create non-persistent entity Main.GetCustomerResponse ();
/

create non-persistent entity Main.Customer (
  Customer__type: string,
  Customer_Value: string,
  CustomerId__type: string,
  CustomerId_Value: string,
  CustomerName__type: string,
  CustomerName_Value: string
);
/

create non-persistent entity Main.Var (
  value: string
);
/
```

### Step 7: Associations (Main)

```sql
-- Auto-created for OdataPlm associations via consumed OData service.
-- Main module associations:
create association Main.Customer_GetCustomerResponse (
  Main.Customer -> Main.GetCustomerResponse
);
/

create association Main.Var_GetCustomerResponse (
  Main.Var -> Main.GetCustomerResponse
);
/
```

### Step 8: Snippet (Entity_Menu)

```sql
create snippet OdataPlm.Entity_Menu {
  dynamictext text1 (content: 'Entities', rendermode: H2)
  navigationlist navigationList1 {
    item (action: show_page 'OdataPlm.Address_Overview') {
      dynamictext text2 (content: 'Address')
    }
    item (action: show_page 'OdataPlm.Customer_Overview') {
      dynamictext text3 (content: 'Customer')
    }
  }
}
```

### Step 9: Pages (OdataPlm)

#### Customer Overview

```sql
create page OdataPlm.Customer_Overview
(title: 'Customer Overview', layout: Atlas_Core.Atlas_Default)
{
  layoutgrid layoutGrid1 {
    row row1 {
      column col1 (desktopwidth: autofill) {
        snippetcall snippetCall1 (snippet: OdataPlm.Entity_Menu)
      }
      column col2 (desktopwidth: autofill) {
        dynamictext text1 (content: 'Customer', rendermode: H2)
        datagrid dataGrid2_1 (datasource: database OdataPlm.Customer) {
          column col1 (attribute: customer_key, caption: 'customer key') {
            textfilter textFilter2
          }
          column col2 (attribute: Industry, caption: 'Industry') {
            textfilter textFilter3
          }
          column col3 (attribute: address_key, caption: 'address key') {
            textfilter textFilter4
          }
          column col4 (attribute: Name, caption: 'Name') {
            textfilter textFilter5
          }
          column col5 (attribute: _Id, caption: 'Id') {
            textfilter textFilter6
          }
          column col6 (attribute: Phone, caption: 'Phone') {
            textfilter textFilter7
          }
          column col7 (attribute: Email, caption: 'Email') {
            textfilter textFilter8
          }
          column col8 (attribute: Contact_Person, caption: 'Contact Person') {
            textfilter textFilter1
          }
          column col9 (attribute: customer_key, ShowContentAs: customContent) {
            actionbutton actionButton1 (action: show_page OdataPlm.Customer_NewEdit, style: primary)
          }
        }
      }
    }
  }
}
```

#### Customer Edit (Popup)

```sql
create page OdataPlm.Customer_NewEdit
(title: 'Edit Customer', layout: Atlas_Core.PopupLayout, params: { $Customer: OdataPlm.Customer })
{
  layoutgrid layoutGrid1 {
    row row1 {
      column col1 (desktopwidth: autofill) {
        dataview dataView1 (datasource: $Customer) {
          textbox textBox1 (label: 'customer key', attribute: OdataPlm.Customer.customer_key)
          textbox textBox2 (label: 'Industry', attribute: OdataPlm.Customer.Industry)
          textbox textBox3 (label: 'address key', attribute: OdataPlm.Customer.address_key)
          textbox textBox4 (label: 'Name', attribute: OdataPlm.Customer.Name)
          textbox textBox5 (label: 'Id', attribute: OdataPlm.Customer._Id)
          textbox textBox6 (label: 'Phone', attribute: OdataPlm.Customer.Phone)
          textbox textBox7 (label: 'Email', attribute: OdataPlm.Customer.Email)
          textbox textBox8 (label: 'Contact Person', attribute: OdataPlm.Customer.Contact_Person)
          combobox comboBox1 (attribute: OdataPlm.Address.address_key)
          footer footer1 {
            actionbutton actionButton1 (caption: 'Save', action: save_changes close_page, style: success)
            actionbutton actionButton2 (caption: 'Cancel', action: cancel_changes close_page)
          }
        }
      }
    }
  }
}
```

#### Address Overview

```sql
create page OdataPlm.Address_Overview
(title: 'Address Overview', layout: Atlas_Core.Atlas_Default)
{
  layoutgrid layoutGrid1 {
    row row1 {
      column col1 (desktopwidth: autofill) {
        snippetcall snippetCall1 (snippet: OdataPlm.Entity_Menu)
      }
      column col2 (desktopwidth: autofill) {
        dynamictext text1 (content: 'Address', rendermode: H2)
        datagrid dataGrid2_1 (datasource: database OdataPlm.Address) {
          column col1 (attribute: address_key, caption: 'address key') {
            textfilter textFilter2
          }
          column col2 (attribute: Country, caption: 'Country') {
            textfilter textFilter3
          }
          column col3 (attribute: State, caption: 'State') {
            textfilter textFilter4
          }
          column col4 (attribute: Street, caption: 'Street') {
            textfilter textFilter5
          }
          column col5 (attribute: Post_Code, caption: 'Post Code') {
            textfilter textFilter6
          }
          column col6 (attribute: City, caption: 'City') {
            textfilter textFilter1
          }
          column col7 (attribute: Zip_Code, caption: 'Zip Code') {
            numberfilter numberFilter1 (filtertype: equal)
          }
          column col8 (attribute: address_key, ShowContentAs: customContent) {
            actionbutton actionButton1 (action: show_page OdataPlm.Address_NewEdit, style: primary)
          }
        }
      }
    }
  }
}
```

#### Address Edit (Popup)

```sql
create page OdataPlm.Address_NewEdit
(title: 'Edit Address', layout: Atlas_Core.PopupLayout, params: { $Address: OdataPlm.Address })
{
  layoutgrid layoutGrid1 {
    row row1 {
      column col1 (desktopwidth: autofill) {
        dataview dataView1 (datasource: $Address) {
          textbox textBox1 (label: 'address key', attribute: OdataPlm.Address.address_key)
          textbox textBox2 (label: 'Country', attribute: OdataPlm.Address.Country)
          textbox textBox3 (label: 'State', attribute: OdataPlm.Address.State)
          textbox textBox4 (label: 'Street', attribute: OdataPlm.Address.Street)
          textbox textBox5 (label: 'Post Code', attribute: OdataPlm.Address.Post_Code)
          textbox textBox6 (label: 'City', attribute: OdataPlm.Address.City)
          textbox textBox7 (label: 'Zip Code', attribute: OdataPlm.Address.Zip_Code)
          footer footer1 {
            actionbutton actionButton1 (caption: 'Save', action: save_changes close_page, style: success)
            actionbutton actionButton2 (caption: 'Cancel', action: cancel_changes close_page)
          }
        }
      }
    }
  }
}
```

#### BOM Tree (Partial — TreeNode widget not configurable)

```sql
-- NOTE: The TREENODE widget cannot be fully configured via MDL.
-- This creates the page structure. Add the TreeNode widget configuration in Studio Pro.
create page OdataPlm.BomTree
(title: 'Bom tree', layout: Atlas_Core.Atlas_Default)
{
  layoutgrid layoutGrid1 {
    row row1 {
      column col1 (desktopwidth: autofill) {
        dynamictext text1 (content: 'BOM', rendermode: H1)
      }
    }
    row row2 {
      column col1 (desktopwidth: autofill) {
        -- TREENODE widget goes here (configure in Studio Pro)
        -- It displays Bom -> Component -> Sub_Component hierarchy
      }
    }
  }
}
```

### Step 10: Home Page (Main)


```sql
create page Main.Home_Web
(title: 'Homepage', layout: Atlas_Core.Atlas_TopBar)
{
  layoutgrid layoutGrid3 {
    row row1 {
      column col1 (desktopwidth: autofill) {
        dynamictext text1 (content: 'Tekst', rendermode: H1)
      }
    }
    row row2 {
      column col1 (desktopwidth: autofill) {
        actionbutton actionButton3 (caption: 'Customer Overview', action: show_page OdataPlm.Customer_Overview)
        actionbutton actionButton4 (caption: 'Bom tree', action: show_page OdataPlm.BomTree)
        datagrid dataGrid2_1 {
          column col1 (attribute: Customer__type, caption: 'Customer type') {
            textfilter textFilter1
          }
          column col2 (attribute: Customer_Value, caption: 'Customer Value') {
            textfilter textFilter2
          }
          column col3 (attribute: CustomerId__type, caption: 'Customer id type') {
            textfilter textFilter3
          }
          column col4 (attribute: CustomerId_Value, caption: 'Customer id Value') {
            textfilter textFilter4
          }
          column col5 (attribute: CustomerName__type, caption: 'Customer name type') {
            textfilter textFilter5
          }
          column col6 (attribute: CustomerName_Value, caption: 'Customer name Value') {
            textfilter textFilter6
          }
          column col7 (attribute: Customer__type, ShowContentAs: customContent) {
            actionbutton actionButton1 (action: show_page Main.Customer_View, style: primary)
            actionbutton actionButton2 (action: delete_object, style: primary)
          }
        }
      }
    }
  }
}
```

### Step 11: Navigation

```sql
create or replace navigation Responsive
  home page Main.Home_Web
  menu (
    menu item 'Home' page Main.Home_Web;
  )
;
```

### Step 12: Microflow (Partial)

```sql
-- NOTE: The RestOperationCallAction is not supported in MDL.
-- This microflow must be completed in Studio Pro by adding the REST call action.
create microflow Main.GetCustomers ()
returns list of Main.Customer as $CustomerList
begin
  -- TODO: Add REST operation call to get customerResponse (requires Studio Pro)
  retrieve $BindingList from association $customerResponse/Main.Customer_GetCustomerResponse;
  return $BindingList;
end;
/
```

## Summary

### What MDL Can Do (~95% of this app)

- Create consumed OData clients with authentication
- Create all external entities linked to the OData client
- Create all domain model entities (external, persistent, non-persistent)
- Create all associations
- Create all overview pages with DataGrid2, text filters, number filters
- Create all edit popup pages with DataView, TextBox, ComboBox, action buttons
- Create the sidebar navigation snippet with NavigationList
- Create the home page with layout grid, buttons, and data grid
- Set up navigation profiles with home pages and menus
- Define constants
- Define enumerations

### What Requires Studio Pro (~5%)

| Gap | Impact | Notes |
|-----|--------|-------|
| **TreeNode widget** | Medium — only affects the BOM tree page | Page structure can be created in MDL; widget needs Studio Pro |
| **RestOperationCallAction** | Low — only used in 1 microflow | The Main.GetCustomers microflow prototype; main app uses OData not REST |

### Recommended Workflow

1. Create the Mendix project in Studio Pro
2. Install marketplace modules (Atlas_Core, Administration, DataWidgets)
3. **With MDL**: Run the scripts above (Steps 2-12) to create the entire app
4. **In Studio Pro**: Add TreeNode widget configuration to BomTree page
5. **In Studio Pro**: Complete the GetCustomers microflow with REST call action (if needed)
