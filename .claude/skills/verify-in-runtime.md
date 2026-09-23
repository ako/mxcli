# Verify a Fix in the Running App

A contributor workflow for proving a fix in a **real Mendix app in a real browser**,
rather than at the BSON or parser layer.

This is the outermost verification tier. It is slow (~5 min the first time, ~2 min
after) and it is not needed for most fixes — read *When This Is Required* before
reaching for it.

## When This Is Required

Use the layer where the symptom actually lives:

| Symptom lives in | Sufficient proof |
|------------------|------------------|
| the parser / grammar | unit test |
| the BSON we write | unit test on the encoded document |
| files on disk after `mx` runs | integration test (`-tags integration`) |
| **the rendered app's behaviour or appearance** | **this skill** |

**The rule: if the symptom is a property of the *running app* rather than of the file
we write, no unit or BSON test can prove the fix.** A page can serialize to perfectly
correct-looking BSON and still render wrong.

## Assert in Text; Screenshot Only When the Question Is Visual

Reaching this skill says the *running app* is the layer. It does not say the answer
has to be a picture, and usually it should not be.

A screenshot costs roughly **1,500 tokens** to read against about **100** for a text
verdict, and the cost is not a one-off: an image read into a conversation is re-read
by every later model call, so one PNG early in a long session is charged hundreds of
times (`docs/11-proposals/PROPOSAL_agent_loop_efficiency.md`).

It is also the *weaker* instrument for the usual question. "Did the page render?",
"is there an error banner?", "did the grid get rows?" are textual, and the commonest
cause of a page that renders blank — a console error — **does not appear in a
picture at all**.

```bash
./mxcli run --local --page-check -p app.mpr        # verdict, no PNG
./mxcli run --local --screenshot -p app.mpr        # PNG *and* verdict
./mxcli playwright verify tests/ -p app.mpr        # assertions; shoots only on failure
```

```
page /p/customers  title="Customers"  h="Customer overview"  rows=12  text=812  console-errors=0
page /p/customers  title="Customers"  NO VISIBLE TEXT  console-errors=1
  ERR    Cannot read properties of undefined (reading 'items')
```

The second line is the blank-page symptom **with its cause named**. That class of bug
took ~40 calls to trace in the session behind the proposal, every one of them looking
at pixels that could not show it.

So:

- **Default to `--page-check` or `playwright verify`.** They answer the question and
  leave the PNG unread.
- **Take a screenshot when the question is genuinely about appearance** — layout,
  spacing, colour, "does this look right" — and then take it **once**, at the end.
- **Never screenshot to confirm something a verdict already reported.** If
  `console-errors=0`, `rows=12` and the heading is right, the page rendered; a picture
  adds cost and no information.

Worked example — mendixlabs/mxcli#812. Every popup opened by an mxcli-authored button
showed a blank caption. The BSON was structurally valid, `mx check` reported 0 errors,
and MxBuild completed. Nothing below the browser could see the defect, because the
defect *was* the rendering: an empty `Microflows$TextTemplate` is not "no title
override", it is an override to the empty string.

Counter-examples from the same week, where this skill would have been waste:
`#808` (MPRv2 → v1 conversion — visible on disk, integration test), `#779` (Commit
flag — visible in BSON round-trip, unit test), mandatory semicolons (parser).

## Getting a database in a container that has none

`run --local` needs PostgreSQL and refuses to boot without one. Two traps in a
dev container, both of which look like "the runtime cannot be verified here":

- **`--ensure-db` fails when no server binary is installed** — the image may
  carry only `postgresql-client`, so `initdb` is absent. The error names
  `initdb`, not the missing package.
- **A forwarded port answers a TCP connect and nothing else.** VS Code forwards
  5432 by default, so `</dev/tcp/host/5432` *succeeds* while `psql` hangs
  forever. Never take a bare port probe as proof a database is there; run an
  actual query.

Docker works if the daemon is up, but the credential helper is often configured
for the HOST (`/opt/homebrew/bin/docker-credential-*`), so any pull dies with
`error getting credentials - err: exit status 255`. Point `DOCKER_CONFIG` at an
empty directory for the command instead of editing the user's config:

```bash
mkdir -p /tmp/dockercfg && echo '{}' > /tmp/dockercfg/config.json
DOCKER_CONFIG=/tmp/dockercfg docker pull postgres:16-alpine
docker run -d --name mxcli-pg -e POSTGRES_USER=mendix -e POSTGRES_PASSWORD=mendix \
  -e POSTGRES_DB=<projectname-lowercased> -p 15432:5432 postgres:16-alpine
```

Then boot with `--db-host 127.0.0.1:15432`. Note `mxcli test` has **no**
`--db-host`: to use a non-default database, boot the app yourself with
`mxcli run --local --test-endpoint --db-host …` and run `mxcli test … --attach`
against it.

## The Procedure

### 1. Cache MxBuild for the version you want

```bash
mxcli setup mxbuild --version 11.12.2
```

Do this **first**, explicitly. Test against the version users report against, not
whatever happens to be cached.

### 2. Create a scratch project — at a SHORT path

```bash
mxcli new PopupDemo --version 11.12.2 --output-dir /root/pd
```

> **Trap: `PathTooLongException`.** MxToolset refuses any full path over **259
> characters** — its own Windows-compatibility limit, not the filesystem's — and
> aborts extraction part way through, leaving ~259 files and no `.mpr`. With the
> blank 11.13 template's longest relative path at 181 characters, the output
> directory gets **77**. A scratchpad path like
> `/tmp/claude-.../<uuid>/scratchpad/proj` blows that on its own.
>
> `mxcli new` handles this since #825 — it creates the project in a short staging
> directory and moves it into place, so any depth works and a failure never leaves
> partial output. It warns when the final path exceeds 259 that Studio Pro on
> Windows may not open the project. **Calling `mx create-project` directly still
> dies**, so keep using a short root (`/root/pd`) for that. The same applies to Go
> tests: set `TMPDIR=/root/t` so `t.TempDir()` stays short, or the scaffolding fails
> and the test **skips** — which is indistinguishable from passing.

Confirm you got the version you asked for; the connect banner prints it:

```bash
echo "" > /root/pd/noop.mdl
mxcli exec /root/pd/noop.mdl -p /root/pd/PopupDemo.mpr | grep -i connected
# Connected to: /root/pd/PopupDemo.mpr (Mendix 11.12.2)
```

### 3. Author the smallest model that shows the symptom

Keep it to the widgets/flows involved. A popup-caption repro is one popup page and one
button:

```mdl
create or replace page MyFirstModule.OrderPopup (
  title: 'Order Details Popup',
  Layout: Atlas_Core.PopupLayout
) { container c1 { dynamictext dtBody (content: 'Body of the popup') } }
/
create or replace page MyFirstModule.Home_Web (
  title: 'Home', Layout: Atlas_Core.Atlas_Default
) {
  container c1 {
    actionbutton btnOpen (caption: 'Open popup', action: show_page MyFirstModule.OrderPopup)
  }
}
/
```

### 4. Provision and boot

```bash
mxcli run --local -p /root/pd/PopupDemo.mpr --setup --ensure-db   # once
mxcli run --local -p /root/pd/PopupDemo.mpr                        # boot (blocks)
```

`--setup` caches the runtime and provisions Postgres without booting. Run the boot in
the background and wait for the ready line rather than sleeping:

```bash
until grep -qE 'App is running at|Error:|Exception' run.log; do sleep 3; done
```

First boot downloads the runtime and bundles the web client (~2–4 min). Later boots
are fast.

### 5. Assert in the browser

Chromium is pre-installed at `/opt/pw-browsers/chromium`; install the Playwright
package only (never `playwright install`):

```bash
PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1 npm install playwright
```

Drive it with `NO_PROXY='*'` so the agent proxy does not intercept localhost. Assert
on **text content**, and print a JSON result so the outcome is unambiguous:

```js
const browser = await chromium.launch({ executablePath: '/opt/pw-browsers/chromium' });
// ... click, wait for the dialog, then:
const title = await dialog.locator('.modal-header .modal-title').innerText();
console.log(JSON.stringify({ captionText: title, captionIsBlank: title.trim() === '' }));
```

### 6. A/B against the broken build — this is the actual proof

**A run that only shows the fixed state proves nothing.** It shows the app works, not
that your change is why. Build a binary from before the fix, rewrite the same model
with it, restart, and re-observe:

```bash
git checkout HEAD~1 -- <the writer files>
go build -o /tmp/mxcli-buggy ./cmd/mxcli
git checkout HEAD -- <the writer files>          # restore immediately

/tmp/mxcli-buggy exec demo.mdl -p app.mpr        # rewrite the model, broken
# restart the app, re-run the browser assertion
```

The #812 result, which is what a finished verification looks like:

| build | `TitleOverride` | `headerText` | caption |
|---|---|---|---|
| fixed | `null` | `"×\nOrder Details Popup"` | `"Order Details Popup"` |
| pre-fix | empty `TextTemplate` | `"×"` | `""` |

Only the second row proves the fix is the cause.

> **Trap: restoring after the mutation.** Use **absolute paths** when copying files
> back. A relative `cp` after a `cd` silently fails and leaves the reverted code in
> your tree — it will be committed. Always finish with `git status`.

## Why This Tier Exists

Two bugs in one week had a green test suite while broken:

- **#812** — a duplicate `codec.RegisterTypeDefaults` for the same `$Type` silently
  clobbered the fix (registrations overwrite rather than merge, resolved by init
  order). The unit test passed; the BSON was still wrong. Caught only by dumping the
  actual document.
- **#808** — the integration test had *only ever skipped*, for want of an `mx` binary.
  A skip reads exactly like a pass in CI output.

So: **a green suite is evidence about the layer it tests, and nothing more.** When a
test can silently become a no-op, prefer `t.Fatal` over `t.Skipf` once the
prerequisite is present — a missing `mx` is a legitimate skip; a present `mx` plus
failed scaffolding is a failure.

## Cleanup

```bash
pkill -f 'mxbuild --serve'; pkill -f runtimelauncher
rm -rf /root/pd
```

Cached MxBuild is ~1.4 GB per version. Keep the versions you test against; drop the
rest if disk gets tight (`~/.mxcli/mxbuild/<version>`).

## Related

- `.claude/skills/fix-issue.md` — symptom table; start there
- `.claude/skills/debug-bson.md` — when the symptom *is* in the BSON
- `.claude/skills/mendix/run-local.md` — full `run --local` reference (flags,
  `--screenshot`, hot reload)
