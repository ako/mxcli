# CRM Module

A complete customer management feature: domain model, validation, CRUD pages, and security -- all in one script.

## Domain Model

```sql
mdl 1;
-- Enumerations first (referenced by entities)
CREATE ENUMERATION CRM.CustomerStatus (
  Active 'Active',
  Inactive 'Inactive',
  Suspended 'Suspended'
);

CREATE ENUMERATION CRM.ContactType (
  Email 'Email',
  Phone 'Phone',
  Visit 'Visit'
);

-- Entities with per-attribute documentation
/** Customer master data */
@Position(100, 100)
CREATE PERSISTENT ENTITY CRM.Customer (
  /** Auto-generated unique identifier */
  CustomerId: AutoNumber NOT NULL UNIQUE DEFAULT 1,
  /** Full legal name */
  Name: String(200) NOT NULL ERROR MESSAGE 'Customer name is required',
  /** Primary contact email */
  Email: String(200) UNIQUE ERROR MESSAGE 'Email already exists',
  /** Phone number in international format */
  Phone: String(50),
  /** Current account balance */
  Balance: Decimal DEFAULT 0,
  /** Whether the account is active */
  IsActive: Boolean DEFAULT TRUE,
  /** Current lifecycle status */
  Status: Enumeration(CRM.CustomerStatus) DEFAULT 'Active',
  /** Free-form notes about this customer */
  Notes: String(unlimited)
)
INDEX (Name)
INDEX (Email);

/** Record of a customer interaction */
@Position(400, 100)
CREATE PERSISTENT ENTITY CRM.ContactLog (
  /** Date and time of the interaction */
  ContactDate: DateTime NOT NULL,
  /** Type of interaction */
  ContactType: Enumeration(CRM.ContactType) DEFAULT 'Email',
  /** Summary of what was discussed */
  Summary: String(2000) NOT NULL ERROR MESSAGE 'Summary is required',
  /** Follow-up needed? */
  FollowUpRequired: Boolean DEFAULT FALSE
);

-- Associations
CREATE ASSOCIATION CRM.ContactLog_Customer
  FROM CRM.ContactLog TO CRM.Customer
  TYPE Reference OWNER Default;

```

## Validation Microflow

The two-microflow pattern: a validation microflow returns field-level feedback, and an action microflow calls it before saving.

```sql
mdl 1;
CREATE MICROFLOW CRM.VAL_Customer ($Customer: CRM.Customer)
RETURNS Boolean AS $IsValid
BEGIN
  DECLARE $IsValid Boolean = true;

  IF trim($Customer/Name) = '' THEN
    SET $IsValid = false;
    VALIDATION FEEDBACK $Customer/Name MESSAGE 'Name cannot be empty';
  END IF;

  IF $Customer/Email != empty AND not(contains($Customer/Email, '@')) THEN
    SET $IsValid = false;
    VALIDATION FEEDBACK $Customer/Email MESSAGE 'Enter a valid email address';
  END IF;

  IF $Customer/Balance < 0 THEN
    SET $IsValid = false;
    VALIDATION FEEDBACK $Customer/Balance MESSAGE 'Balance cannot be negative';
  END IF;

  RETURN $IsValid;
END;

CREATE MICROFLOW CRM.ACT_Customer_Save ($Customer: CRM.Customer)
RETURNS Boolean AS $IsValid
BEGIN
  $IsValid = CALL MICROFLOW CRM.VAL_Customer(Customer = $Customer);

  IF $IsValid THEN
    COMMIT $Customer;
    CLOSE PAGE;
  END IF;

  RETURN $IsValid;
END;
```

## Pages

```sql
mdl 1;
-- NewEdit page with validation (first: the overview opens it)
CREATE PAGE CRM.Customer_NewEdit (
  Params: ( $Customer: CRM.Customer ),
  Title: 'Customer',
  Layout: Atlas_Core.PopupLayout
) {
  LAYOUTGRID mainGrid {
    ROW {
      COLUMN (DesktopWidth: AutoFill) {
        DATAVIEW dataView1 (DataSource: $Customer) {
          TEXTBOX txtName (Label: 'Name', Attribute: Name)
          TEXTBOX txtEmail (Label: 'Email', Attribute: Email)
          TEXTBOX txtPhone (Label: 'Phone', Attribute: Phone)
          TEXTAREA txtNotes (Label: 'Notes', Attribute: Notes)
          FOOTER {
            ACTIONBUTTON btnSave (
              Caption: 'Save',
              Action: CALL MICROFLOW CRM.ACT_Customer_Save,
              ButtonStyle: Success
            )
            ACTIONBUTTON btnCancel (Caption: 'Cancel', Action: CANCEL CHANGES)
          }
        }
      }
    }
  }
};

-- Overview page with data grid
CREATE PAGE CRM.Customer_Overview (
  Title: 'Customers',
  Layout: Atlas_Core.Atlas_Default
) {
  DATAGRID dgCustomers (DataSource: DATABASE CRM.Customer, Selection: Single) {
    COLUMN (Attribute: Name, Caption: 'Name') { TEXTFILTER fName }
    COLUMN (Attribute: Email, Caption: 'Email') { TEXTFILTER fEmail }
    COLUMN (Attribute: Phone, Caption: 'Phone')
    COLUMN (Attribute: Status, Caption: 'Status')
    COLUMN (Attribute: IsActive, Caption: 'Active')
    CONTROLBAR {
      ACTIONBUTTON btnNew (Caption: 'New', Action: CREATE OBJECT CRM.Customer THEN SHOW PAGE CRM.Customer_NewEdit, ButtonStyle: Primary)
    }
  }
};

```

## Security

```sql
mdl 1;
-- Module roles
CREATE MODULE ROLE CRM.User;
CREATE MODULE ROLE CRM.Admin DESCRIPTION 'Full customer management access';

-- Entity access
GRANT CREATE, DELETE, READ *, WRITE * ON ENTITY CRM.Customer TO CRM.Admin;
GRANT CREATE, READ *, WRITE * ON ENTITY CRM.Customer TO CRM.User
  WHERE [IsActive = true];

GRANT CREATE, DELETE, READ *, WRITE * ON ENTITY CRM.ContactLog TO CRM.Admin;
GRANT CREATE, READ *, WRITE * ON ENTITY CRM.ContactLog TO CRM.User;

-- Document access
GRANT EXECUTE ON MICROFLOW CRM.ACT_Customer_Save TO CRM.User;
GRANT VIEW ON PAGE CRM.Customer_Overview TO CRM.User;
GRANT VIEW ON PAGE CRM.Customer_NewEdit TO CRM.User;

-- User roles
CREATE OR MODIFY USER ROLE CRMUser ( ModuleRoles: (System.User, CRM.User) );
CREATE OR MODIFY USER ROLE CRMAdmin ( ModuleRoles: (System.User, CRM.Admin) );

-- Demo users for testing
CREATE OR MODIFY DEMO USER 'crm_user' ( Password: 'CrmPassword1!', UserRoles: (CRMUser) );
CREATE OR MODIFY DEMO USER 'crm_admin' ( Password: 'CrmPassword1!', UserRoles: (CRMAdmin) );
ALTER APP SECURITY ( EnableDemoUsers: TRUE );
```
