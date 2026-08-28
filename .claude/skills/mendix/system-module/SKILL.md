---
name: system-module
description: "Reference for the built-in System module — User, FileDocument, Image, workflow and queue entities, and the associations you are allowed to make to them. Use when linking a record to System.User, handling file uploads, or working with workflow entities."
---

# Mendix System Module Reference

The `System` module is a built-in Mendix module present in every application. It provides core entities for user management, file handling, workflows, task queues, HTTP services, and more. These entities are not defined in the application's domain model but are available for use in microflows, pages, associations, and Java actions.

## Access to System entities has a ceiling

**No project module can widen access to `System.User`, `System.Workflow` or
`System.WorkflowUserTask`.** Their access comes from the System module's own roles,
and a `grant` in your module cannot raise it — so any UI over them is
Administrator-only unless the data is denormalised into your own entities or reached
through a microflow data source (microflows bypass entity access by default).

It fails **silently**: a combo box over `System.User` lists the current user only, a
grid over `System.Workflow` renders empty, and both `mx check` and `mxcli lint` pass.
See the System-module ceiling section in [manage-security](../manage-security/SKILL.md)
for the three ways around it.

## Reference files

- [`reference/workflow-and-queues.md`](reference/workflow-and-queues.md) — the
  workflow engine's entities (`WorkflowInstance`, `UserTask`, the state
  enumerations and how they relate), plus task queues and scheduled events. Open
  it when querying workflow state or wiring anything to a queue.

## When to Use This Skill

Use this skill when:
- Creating associations to System entities (e.g., linking a record to `System.User`)
- Working with file uploads/downloads (`System.FileDocument`, `System.Image`)
- Implementing workflow-related features
- Writing microflows that reference System types
- Understanding what's available out of the box in Mendix
- Designing security models that reference `System.User` and `System.UserRole`

## How to Reference System Entities

In MDL, reference System entities with the `System.` prefix:

```mdl
-- Association to the current user
create association MyModule.Order_CreatedBy
from MyModule.Order to System.User
type reference;

-- Entity that generalizes FileDocument for file uploads
create persistent entity MyModule.Invoice extends System.FileDocument (
  InvoiceNumber: string(50),
  IssueDate: datetime
);

-- Entity that generalizes Image for image uploads
create persistent entity MyModule.ProductPhoto extends System.Image (
  PhotoCaption: string(200),
  IsPrimary: boolean default false
);
```

---

## 1. User Management & Authentication

### System.User

The central user entity. All application users are instances of `System.User` or a specialization (e.g., `Administration.Account`).

| Attribute | Type | Description |
|-----------|------|-------------|
| Name | String | Username (login name) |
| Password | String | Hashed password (write-only) |
| LastLogin | DateTime | Timestamp of last successful login |
| Blocked | Boolean | Whether user account is blocked |
| BlockedSince | DateTime | When the account was blocked |
| Active | Boolean | Whether the account is active |
| FailedLogins | Integer | Count of consecutive failed login attempts |
| WebServiceUser | Boolean | Whether this is a web service account |
| IsAnonymous | Boolean | Whether this is an anonymous user |

| Association | Target | Type | Description |
|-------------|--------|------|-------------|
| User_UserRoles | System.UserRole | Many-to-Many | Roles assigned to this user |
| User_Language | System.Language | Many-to-One | User's preferred language |
| User_TimeZone | System.TimeZone | Many-to-One | User's timezone |

**Common usage patterns:**
- Associate application entities to `System.User` for audit trails (CreatedBy, ModifiedBy)
- Specialize `System.User` (via `Administration.Account`) to add custom profile attributes
- Use `[%CurrentUser%]` token in XPath to filter by logged-in user
- Check `$user/Active` and `$user/Blocked` for access control

### System.UserRole

Represents an application user role (e.g., Administrator, User, Manager).

| Attribute | Type | Description |
|-----------|------|-------------|
| ModelGUID | String | GUID from the model definition |
| Name | String | Role name as defined in the model |
| Description | String | Role description |

| Association | Target | Type | Description |
|-------------|--------|------|-------------|
| UserRole_GrantableRoles | System.UserRole | Many-to-Many | Roles that holders of this role can assign to others |

### System.Session

Active user sessions.

| Attribute | Type | Description |
|-----------|------|-------------|
| SessionId | String | Unique session identifier |
| CSRFToken | String | Cross-site request forgery token |
| LastActive | DateTime | Last activity timestamp |

| Association | Target | Type | Description |
|-------------|--------|------|-------------|
| Session_User | System.User | Many-to-One | User who owns this session |

### System.Language

Available languages for internationalization.

| Attribute | Type | Description |
|-----------|------|-------------|
| Code | String | Language code (e.g., `en_US`, `nl_NL`) |
| Description | String | Human-readable language name |

### System.TimeZone

Available time zones.

| Attribute | Type | Description |
|-----------|------|-------------|
| Code | String | Timezone identifier (e.g., `Europe/Amsterdam`) |
| Description | String | Human-readable timezone name |
| RawOffset | Integer | Offset from UTC in milliseconds |

### System.TokenInformation

Authentication tokens (e.g., for "remember me" or API tokens).

| Attribute | Type | Description |
|-----------|------|-------------|
| Token | String | Hashed token value (write-only) |
| ExpiryDate | DateTime | When the token expires |
| UserAgent | String | Browser/client user agent |

| Association | Target | Type | Description |
|-------------|--------|------|-------------|
| TokenInformation_User | System.User | Many-to-One | User who owns this token |

---

## 2. File Management

### System.FileDocument

Base entity for all file storage. Specialize this entity to create custom file types.

| Attribute | Type | Description |
|-----------|------|-------------|
| FileID | Long | Internal file identifier |
| Name | String | File name (including extension) |
| DeleteAfterDownload | Boolean | Auto-delete after first download |
| Contents | Binary | The file binary content |
| HasContents | Boolean | Whether file content has been uploaded |
| Size | Long | File size in bytes |

**Usage:** Create a specialization to store typed files:

```mdl
create persistent entity MyModule.Attachment extends System.FileDocument (
  description: string(500),
  Category: MyModule.AttachmentCategory
);

create association MyModule.Order_Attachments
from MyModule.Order to MyModule.Attachment
type reference_set;
```

### System.Image

Extends `System.FileDocument` with image-specific features.

| Attribute | Type | Description |
|-----------|------|-------------|
| PublicThumbnailPath | String | Path to auto-generated thumbnail |
| EnableCaching | Boolean | Whether the browser should cache this image |

**Usage:** Specialize for application images:

```mdl
create persistent entity MyModule.ProductPhoto extends System.Image (
  PhotoCaption: string(200),
  SortOrder: integer default 0
);
```

### System.SynchronizationErrorFile

File attachment for offline synchronization errors. Extends `System.FileDocument`.

---

## 3. HTTP / Web Services

### System.HttpMessage (base, non-persistent)

Base entity for HTTP messages. Not stored in the database.

| Attribute | Type | Description |
|-----------|------|-------------|
| HttpVersion | String | HTTP version (e.g., `1.1`) |
| Content | String | Message body content |

### System.HttpRequest (extends HttpMessage, non-persistent)

| Attribute | Type | Description |
|-----------|------|-------------|
| Uri | String | Request URI |

### System.HttpResponse (extends HttpMessage, non-persistent)

| Attribute | Type | Description |
|-----------|------|-------------|
| StatusCode | Integer | HTTP status code (200, 404, 500, etc.) |
| ReasonPhrase | String | Status reason phrase |

### System.HttpHeader (non-persistent)

| Attribute | Type | Description |
|-----------|------|-------------|
| Key | String | Header name |
| Value | String | Header value |

| Association | Target | Type | Description |
|-------------|--------|------|-------------|
| HttpHeaders | System.HttpMessage | Many-to-One | Parent HTTP message |

**Usage:** These entities are used in published/consumed REST services and Java actions that handle HTTP requests and responses directly.

### System.ConsumedODataConfiguration

Configuration for consumed OData services.

| Attribute | Type | Description |
|-----------|------|-------------|
| ServiceUrl | String | OData service endpoint URL |
| ProxyConfiguration | Enum (ProxyConfiguration) | Proxy setting: UseAppSettings, Override, NoProxy |
| ProxyHost | String | Proxy hostname (when Override) |
| ProxyPort | Integer | Proxy port (when Override) |
| ProxyUsername | String | Proxy authentication username |
| ProxyPassword | String | Proxy authentication password |

### System.ODataResponse

| Attribute | Type | Description |
|-----------|------|-------------|
| Count | Long | Total record count from OData response |

---

## 4. Error Handling

### System.Error (non-persistent)

| Attribute | Type | Description |
|-----------|------|-------------|
| ErrorType | String | Error category/type |
| Message | String | Error message |
| Stacktrace | String | Full stack trace |

### System.SoapFault (extends Error, non-persistent)

SOAP-specific fault information. Extends `System.Error` with SOAP fault details.

### System.SynchronizationError

Tracks offline mobile synchronization failures.

| Attribute | Type | Description |
|-----------|------|-------------|
| Reason | String | Why synchronization failed |
| ObjectId | String | ID of the object that failed |
| ObjectType | String | Entity type of the failed object |
| ObjectContent | String | Serialized state of the object |

---

## 7. Utility Entities

### System.Paging (non-persistent)

Paging information for data retrieval in custom Java actions.

| Attribute | Type | Description |
|-----------|------|-------------|
| PageNumber | Long | Current page number |
| IsSortable | Boolean | Whether sorting is supported |
| SortAttribute | String | Attribute to sort by |
| SortAscending | Boolean | Sort direction |
| HasMoreData | Boolean | Whether more pages exist |

### System.UserReportInfo

Information for user management reports (internal use).

### System.ProxyConfiguration

HTTP proxy settings (internal use).

---

## 8. Enumerations

### WorkflowState
`InProgress`, `Paused`, `Completed`, `Aborted`, `Incompatible`, `Failed`

### WorkflowUserTaskState
`created`, `InProgress`, `Completed`, `Paused`, `Aborted`, `Failed`

### WorkflowUserTaskCompletionType
`single`, `Veto`, `Consensus`, `Majority`, `Threshold`, `microflow`

### WorkflowActivityType
`Start`, `end`, `ExclusiveSplit`, `ParallelSplit`, `ParallelSplitBranchStopper`, `ParallelSplitMerge`, `UserTask`, `CallMicroflow`, `CallWorkflow`, `JumpTo`, `MultiInputUserTask`, `WaitForNotification`, `WaitForTimer`, `EndOfBoundaryEventPath`, `NonInterruptingTimerEvent`, `InterruptingTimerEvent`

### WorkflowActivityExecutionState
`created`, `InProgress`, `Completed`, `Paused`, `Aborted`, `Failed`

### WorkflowCurrentActivityAction
`DoNothing`, `JumpTo`

### WorkflowEventType
`WorkflowCompleted`, `WorkflowInitiated`, `WorkflowRestarted`, `WorkflowFailed`, `WorkflowAborted`, `WorkflowPaused`, `WorkflowUnpaused`, `WorkflowRetried`, `WorkflowUpdated`, `WorkflowUpgraded`, `WorkflowConflicted`, `WorkflowResolved`, `WorkflowJumpToOptionApplied`, `StartEventExecuted`, `EndEventExecuted`, `DecisionExecuted`, `JumpExecuted`, `ParallelSplitExecuted`, `ParallelMergeExecuted`, `CallWorkflowStarted`, `CallWorkflowEnded`, `CallMicroflowStarted`, `CallMicroflowEnded`, `WaitForNotificationStarted`, `WaitForNotificationEnded`, `WaitForTimerStarted`, `WaitForTimerEnded`, `UserTaskStarted`, `MultiUserTaskOutcomeSelected`, `UserTaskEnded`, `NonInterruptingTimerEventExecuted`, `InterruptingTimerEventExecuted`

### QueueTaskStatus
`Idle`, `Running`, `Completed`, `Failed`, `Retrying`, `Aborted`, `Incompatible`

### EventStatus
`Running`, `Completed`, `error`, `Stopped`

### ContextType
`System`, `user`, `Anonymous`, `ScheduledEvent`

### UserType
`Internal`, `external`

### DeviceType
`Phone`, `Tablet`, `Desktop`

---

## 9. Inheritance Hierarchies

```
System.User
  └── Administration.Account (adds FullName, Email, etc.)

System.FileDocument
  ├── System.Image
  └── System.SynchronizationErrorFile

System.HttpMessage (non-persistent)
  ├── System.HttpRequest
  └── System.HttpResponse

System.Error (non-persistent)
  └── System.SoapFault
```

**Key point for MDL:** When creating entities that store files or images, use `extends`:

```mdl
create persistent entity MyModule.Document extends System.FileDocument (
  title: string(200),
  version: integer default 1
);

create persistent entity MyModule.Photo extends System.Image (
  AltText: string(200)
);
```

## 10. Common Patterns

### Audit Trail (CreatedBy / ModifiedBy)

```mdl
create association MyModule.Order_CreatedBy
from MyModule.Order to System.User
type reference;

create association MyModule.Order_ModifiedBy
from MyModule.Order to System.User
type reference;
```

### File Attachments

```mdl
create persistent entity MyModule.Attachment extends System.FileDocument (
  description: string(500)
);

create association MyModule.Order_Attachments
from MyModule.Order to MyModule.Attachment
type reference_set;
```

### Workflow Context Object

```mdl
-- Application entity that serves as workflow context
create persistent entity MyModule.ExpenseReport (
  Amount: decimal,
  description: string(500),
  status: MyModule.ApprovalStatus default 'Draft'
);

-- Associate with workflow instance
create association MyModule.ExpenseReport_Workflow
from MyModule.ExpenseReport to System.Workflow
type reference;
```

### Task Inbox Page

User tasks can be displayed in pages by retrieving `System.WorkflowUserTask` where the current user is in the target users or assignees.

### XPath Tokens for System Entities

```
[%CurrentUser%]     -- The logged-in System.User
[%CurrentObject%]   -- The current context object
```

## Source

This reference was extracted from the Java proxy files in `javasource/system/proxies/` of a Mendix 10.x application. The System module domain model is not exposed in MPR/BSON files but is defined internally by the Mendix runtime. Proxy files serve as the definitive reference for available entities, attributes, and associations.
