# Status codes, capabilities and authentication

Supporting reference for [odata-data-sharing](../SKILL.md).

## HTTP Status Codes and Errors: What Each Capability Can Do

**The read path and the write path have different powers, and the difference is
the single most expensive thing to get wrong here.** Read this before designing
any microflow-backed resource.

| Capability | Can set the HTTP status code? | How |
|---|---|---|
| OData **action** (published microflow) | **Yes** | add a `System.HttpResponse` parameter |
| Entity **Insertable / Updatable / Deletable** microflow | **Yes** | add a `System.HttpResponse` parameter |
| Entity **Readable** microflow | **No** | not offered — the read capability has no documented `HttpResponse` parameter |

Sources: [published-odata-microflow §4](https://docs.mendix.com/refguide/published-odata-microflow/#4-customizing-the-outgoing-http-response),
[published-odata-entity, custom HTTP response](https://docs.mendix.com/refguide/published-odata-entity/#custom-http-response).
The custom-response section names Insertable, Updatable and Deletable; the
Readable section never references it.

### Writing a status code (action / insert / update / delete)

```sql
create microflow Api.InsertRow (
  $Row: Api.Row,
  $HttpResponse: System.HttpResponse
)
begin
  if $Row/RowKey = empty then
    change $HttpResponse (StatusCode = 400, Content = '{"error":"rowKey is required"}');
    return;
  end if;
  ...
end;
```

Three rules the platform imposes:

- **`ReasonPhrase` is ignored.** Setting it is dead code; put the explanation in
  `Content`.
- **`204` always produces an empty body.** Setting `Content` alongside it is
  discarded.
- **Changing status or content makes the whole response come from
  `HttpResponse`** — headers included. Changing *only* headers merges them with
  the defaults instead.
- `Transfer-Encoding` and `Date` cannot be changed.

### The read path cannot refuse, so it must not over-promise

A read microflow has no way to answer `400`. Its only exits are to throw (a
blunt `500`) or to return data. That has two consequences, and both are design
obligations rather than nice-to-haves:

**1. Declare capabilities you do not implement as `No`.** Mendix applies *no*
query options to a read-microflow resource — it hands over the request and
returns whatever comes back — so `TopSupported` / `SkipSupported` / `Countable`
are claims about your microflow, not about the platform. A resource that
advertises `TopSupported: Yes` and ignores `$top` returns the entire collection
with a `200`, and the client believes it received a page.

```sql
publish entity Api.Row as 'Rows' (
  ReadMode: microflow Api.Read_Rows,
  -- Only claim what Read_Rows actually parses out of the URI:
  TopSupported: No,
  SkipSupported: No,
  Countable: No
)
```

Declaring `No` is the read path's substitute for the `400` it cannot send. It is
also safe to declare: mxcli reads these capabilities off the contract, so a
consuming app generated from this service says `No` too. (Until it learned to,
the generator always said `Yes`, and the consumer failed to build with **CE6630**
"'Rows' is marked supports $top=False in the OData service, but True in the app"
— if you hit that against an older mxcli, that is the cause.)

**The capabilities vocabulary has two annotation shapes**, and it is worth knowing
which you are looking at when reading a `$metadata` by hand:

| Capability | Shape |
|---|---|
| `TopSupported`, `SkipSupported` | standalone: `<Annotation Bool="false" Term="…TopSupported"/>` |
| `Insertable`, `Updatable`, `Deletable`, `Countable` | a `<Record>` with a `Bool` property value |
| `Filterable`, `Sortable` | **either** — a record listing `NonFilterableProperties` when *some* attributes are filterable, or a bare `Bool="false"` on the record when *none* are |

That last row is the trap. Mendix picks the shape by arithmetic (there is no list
to write when nothing is filterable), so both appear in one document on different
entity sets, and a service where an entity exposes only a KEY emits the whole-set
form.

`mxcli check` enforces this as **MDL-ODATA03**, and it looks for the option
*names* in the microflow body, not just for a `System.HttpRequest` parameter —
adding the parameter to answer the KEY (below) does not silence the paging
warning. A microflow that hands the request to a Java action, a JavaScript
action, or a microflow outside the script is left alone, since mxcli cannot read
what those do.

**2. Answer a lookup by your own KEY.** A client holding a row re-reads it by
key, unprompted, and Mendix's own OData client sends the `$filter` spelling:

```
?$filter=rowKey eq '1036-c'     ← what the runtime actually sends
/Rows('1036-c')                 ← bare path key
/Rows(rowKey='1036-c')          ← named path key
```

If the microflow parses only its collection filter, the key request falls through
to the collection default and the client adopts the **first row** as the identity
of the object it is displaying. There is no error: the request is well-formed,
the response is a valid collection, the count is right, the status is `200`. Two
different objects are then on screen at once, and nothing distinguishes them
until one travels to another page.

So: `expose ( … (KEY) )` is a promise the *service* makes on the *microflow's*
behalf. Branch key → id → filter → default.

**Not declaring the KEY is not a way out.** Mendix requires a published entity to
have one — `CE6585 "Published entity 'X' must have a key defined."` — so the only
correct resolution is to answer the lookup. (Query *options* you may decline;
the key you may not.)

The request itself always arrives on `System.HttpRequest`:

```sql
create microflow Api.Read_Rows (
  $Request:  System.HttpRequest,
  $Response: System.ODataResponse    -- required while Countable is Yes
)
returns List of Api.Row
begin
  log info 'URI=' + $Request/Uri;    -- the whole query string, URL-encoded
  ...
end;
```

To watch what clients actually send, raise the **`OData Publish`** log node (note
the space; it exists only when the project publishes a service):

```bash
mxcli log set "OData Publish" TRACE
```
```
TRACE - OData Publish: Incoming request from 127.0.0.1: GET .../Rows?$top=5&$filter=rowKey eq 'abc'
DEBUG - OData Publish: Responding to client with status code 400.
```

That is the fastest way to see a client re-reading a row by key, and it needs no
change to the model. `ODataConsume` is the client side, a different node.

Mendix validates field names before the microflow runs (`$filter=secretColumn eq 'x'`
is a `400` from the platform), and it enforces `Filterable`: filtering on a property
you did not declare filterable is rejected with `400 "Property 'x' is
non-filterable"` before the microflow runs. So the microflow only ever sees names
that exist in the published metadata. That is defence in depth, not a substitute for a
whitelist — it constrains the *name*, not what you do with it.
## Authentication Methods, and the Cost of Basic Auth

A published service names one or more methods in the `authentication` clause:

```sql
authentication basic, session
authentication microflow ProductApi.Authenticate
```

| Method | How the caller proves itself | Cost per request |
|---|---|---|
| `basic` | `Authorization: Basic …` on every request | **a full password hash** |
| `session` | an existing session + `X-Csrf-Token` | none, but only reachable from same-origin JavaScript in the same app |
| `guest` | anonymous | none |
| `microflow` | your microflow decides | whatever the microflow does |

**Basic auth hashes on every call, including failures.** A consumed OData
service holds no session, so every request is a fresh login. Measured on a real
app, that was 60–80% of a page turn — and a *wrong* password costs the same,
because the hash runs either way:

```
GET /odata/f1/Drivers?$top=20
  basic auth                 200  ~360 ms
  wrong password             401  ~346 ms   <- hashes either way
  session cookie             200   ~19 ms
  no credentials             401   ~42 ms
```

Do not read the 19 ms as the fix: `session` is only open to same-origin
JavaScript, and a Mendix OData *client* sends basic auth and keeps no cookie.

### Custom authentication

A microflow taking `List of System.HttpHeader` and returning a `System.User`.
Return empty to deny. No password hash anywhere, so the ~42 ms floor.

```sql
CREATE MICROFLOW ProductApi.Authenticate ($Headers: List of System.HttpHeader)
  RETURNS System.User AS $User
BEGIN
  -- e.g. compare a shared secret from $Headers, then retrieve the service account
  retrieve $Users from System.User;
  $User = head($Users);
  RETURN $User;
END;

create odata service ProductApi.Api ( ... )
authentication microflow ProductApi.Authenticate
{ ... };
```

**The client needs no change.** The microflow can read the `Authorization:
Basic …` header itself and compare it against a constant, so the consumer keeps
its existing username/password configuration and the server simply stops
calling BCrypt.

Two build rules to know before you reach for it:

- **The microflow is mandatory.** `authentication microflow` with no name parses
  but fails the build with **CE0333** "Please select a microflow to use for
  authentication". `mxcli check` flags this as `MDL-ODATA04`.
- **App security must be on.** With security off, Mendix reports **CE6600**
  "App security is off, but custom authentication is enabled for this service".
  Set it with `alter project security level prototype` (or `production`).

If custom authentication is more than you need, `ALTER SETTINGS MODEL
BcryptCost = 8` shrinks the hash instead of removing it. Each step down halves
the cost; the default is 12. That is a judgement about the accounts in *that*
app — appropriate for machine accounts with generated passwords, not for a
database of human passwords.
## Advanced: Configuration Microflow for Custom Headers

When the consumer needs to pass custom headers (e.g., for audit trails or user context), use a configuration microflow:

```sql
/**
 * Adds current user name as custom header for audit logging.
 */
create microflow ProductClient.SetClientHeaders (
  $httpResponse: System.HttpResponse
)
returns list of System.HttpHeader as $HttpHeaderList
begin
  $HttpHeaderList = create list of System.HttpHeader;
  $NewHttpHeader = create System.HttpHeader (
    key = 'X-Audit-User',
    value = $currentUser/Name
  );
  add $NewHttpHeader to $HttpHeaderList;
  return $HttpHeaderList;
end;
```

Reference it in the client:

```sql
create odata client ProductClient.ProductDataApiClient (
  ...
  ConfigurationMicroflow: microflow ProductClient.SetClientHeaders
);
```
