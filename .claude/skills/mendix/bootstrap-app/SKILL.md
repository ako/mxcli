---
name: bootstrap-app
description: "Provision a Mendix project in a repo that has none — interview, `mxcli new`, session hook, project brief, first commit and boot. Use when the repository is empty or has no .mpr yet, typically from the empty-repo seed prompt."
---

# Bootstrap a Mendix App in an Empty Repo

## When to Use This Skill

Use this when a repo has **no Mendix project yet** and you have been asked to
provision one with mxcli — typically from the empty-repo seed prompt, which does
nothing but install mxcli, run `mxcli init --sync-skills`, and send you here.

Everything the seed prompt used to spell out lives here instead, so the prompt stays
short enough to paste from a phone and this procedure can be fixed by shipping a new
mxcli rather than by re-pasting a longer prompt.

If an `.mpr` already exists, this is the wrong skill: run `mxcli init` in the app
folder and go straight to the work.

Related skills: `run-local` (the warm dev loop this ends in), `mdl-entities` and
`create-page` (building the model you propose at the end),
`migrate-design-prototype` (when a design was handed to you).

---

## Step 0 — interview, and WAIT for the answers

Ask **all** of these in ONE message, numbered, each with the default you would pick,
so the user can reply "defaults" or answer only what they care about. **Do not
provision anything until they reply.**

The interview comes first for a reason: the app name becomes the `.mpr` file name, the
Studio Pro app name and the path baked into the SessionStart hook, so it is far
cheaper to ask than to rename afterwards. The rest of the answers are the brief — they
get written into the repo, so the session that resumes after an idle reap knows what
it is building.

1. **One app, or a solution of several?** One Mendix app is the default. "Solution"
   means several apps in one repo — e.g. a backend that owns the data and publishes
   OData/REST, and a frontend that consumes it. If so, ask for each app's name and one
   line on what it owns, and follow "If this is a solution" below.
2. **App name.** Becomes the `.mpr` file name, the app name in Studio Pro, and the
   path in the session hook, so it is awkward to change later. One PascalCase word,
   letters and digits only — `OrderPortal`, `FieldService`, `ClubAdmin`. Propose one
   derived from the answer to Q3.
3. **What is the app for?** One or two sentences: who uses it, and what it lets them
   do. If the answer is vague ("a tool for work"), ask one follow-up — everything
   below is derived from this.
4. **What does it keep track of?** Three to six nouns that will become entities, and a
   word on how they relate (e.g. "a Job has many Visits; each Visit has Photos"). For
   a solution, also ask which app owns each noun.
5. **Who logs in?** The user roles, and roughly what each may do (e.g. "Requester
   creates and sees their own; Approver sees everything and approves").
6. **Look and feel.** One of the bundled themes: `signal` (light, high contrast),
   `ledger` (light, dense, data-heavy), `console` (dark), or `none` for stock Atlas.
   Default `signal`.
7. **Mendix version.** Default `11.13.0`.

If the user says "defaults" or ignores a question, choose something sensible for it,
say what you chose in one line, and keep going — **do not block on them twice**.

### Checking the Mendix version default

Everything mxcli does starts with downloading MxBuild, so "supported" means "on the
CDN". If asked for a version newer than the default, verify both tarballs answer
`200` before using it — `run --local` needs the runtime as well as MxBuild:

```bash
curl -sI -o /dev/null -w '%{http_code}\n' https://cdn.mendix.com/runtime/mxbuild-11.13.0.tar.gz
curl -sI -o /dev/null -w '%{http_code}\n' https://cdn.mendix.com/runtime/mendix-11.13.0.tar.gz
```

In a solution, give every app the **same** version: they share the `~/.mxcli/mxbuild`
cache, and a mismatch means a second multi-hundred-MB download and two runtimes to
keep straight.

---

## Provision

Substitute the answers for `<AppName>`, `<version>` and `<theme>` throughout. For a
solution, do steps 1–3 once per app and read "If this is a solution" first. The
commands below say `./mxcli` because that is where the seed prompt puts the binary —
drop the `./` if it came pre-installed on `PATH`.

1. **Create the app at the repo root** — that is where `.claude/` and the `./mxcli`
   binary have to live for future sessions to self-bootstrap. `mxcli new` refuses to
   write into a directory that is not empty, and a git repo always has `.git`, so
   create it in a subfolder and move it up:

   ```bash
   ./mxcli new <AppName> --version <version> --theme <theme>
   rm -f <AppName>/mxcli        # a hardlink to the ./mxcli you just ran; mv would
                                # refuse it as "the same file". Still needed when
                                # mxcli is already on PATH — `new` hardlinks it
                                # into the project either way.
   rm -rf .ai-context           # the seed prompt's own `mxcli init --sync-skills`
                                # wrote this; `mxcli new` writes its own copy from
                                # the same binary, so the one here is a stale
                                # duplicate and `mv` fails with
                                #   mv: cannot overwrite './.ai-context': Directory not empty
   shopt -s dotglob && mv <AppName>/* . && rmdir <AppName>
   ```

   `mxcli new` also runs `mxcli init`, which writes `.claude/settings.json` with a
   SessionStart hook plus the `.claude/bootstrap-mxcli.sh` it runs — **check that the
   `.mpr` named in the script is right after the move.**
2. **Confirm the Claude tooling:** `./mxcli init --tool claude`. Idempotent — it is
   what step 1 already ran, and re-running it is the cheapest way to be sure the hook,
   skills and commands are in place.
3. **Bring prerequisites up:** `./mxcli run --local --setup --ensure-db -p <AppName>.mpr`
   (caches MxBuild + runtime, starts Postgres, creates the app database).
4. **Write the brief to `README.md`** at the repo root: the app name(s), the answers
   to Q3–Q5 **in the user's words**, and the theme and Mendix version used. For a
   solution, say which app owns what and how they talk to each other. This is what
   tells the next session — after an idle reap, with none of this conversation — what
   it is building. Keep it short enough that it stays true.
5. **Start a `FINDINGS.md`** at the repo root and keep appending to it as you work.
   Log anything surprising or broken: an mxcli command that errored, a workaround you
   applied, a `mxcli check` that passed but a real `mx check` later flagged. Note the
   Mendix + mxcli versions and how each finding was verified. This is durable context
   for the next session, and the most useful thing to share back to improve mxcli.
6. **COMMIT everything now** — `<AppName>.mpr`, `.devcontainer/`, `.claude/` (the
   SessionStart hook **and** `.claude/bootstrap-mxcli.sh`), `README.md` and
   `FINDINGS.md`. This step is mandatory, not housekeeping: the seed prompt is a
   *one-time* seed, and committing its output is what makes every later session
   bootstrap from files instead of from a re-paste. The `mxcli` binary itself stays
   git-ignored (~85 MB); the bootstrap script is what fetches it back into a fresh
   clone, so committing the script is what makes the hook survive a reap.
7. **Boot and verify:** `./mxcli run --local -p <AppName>.mpr` in the background, then
   confirm the app answers HTTP 200 at http://localhost:8080/ and report.
8. **(Optional) browser preview from a cloud session:**
   `./mxcli run --hub https://hub.mxcli.org -p <AppName>.mpr`, and report the preview
   URL it prints. Needs `MXCLI_HUB_KEY` on the environment; without it, continue as a
   normal local run. `--hub` ships in the **Linux** build only (a cloud session is a
   Linux container, so it works there); on a native Windows/macOS mxcli it fails with
   an explanatory message — continue as a normal local run.

---

## If this is a solution (several apps in one repo)

Each app is a full Mendix project — one `.mpr`, one runtime, one database. Same steps,
with these deltas:

- **Layout.** One subfolder per app, nothing at the repo root but `README.md`,
  `FINDINGS.md` and `.claude/`. Run `mxcli new <AppName> --version <version> --theme
  <theme>` once per app and leave each where it lands; do not move anything up.
- **Ports.** Every app defaults to 8080/8090/6543 and they will collide. Give the
  first app the defaults and the second `--app-port 8180 --admin-port 8190
  --serve-port 6643`. Avoid 8081/8091/6544 — `mxcli test --local` uses those.
- **Give each app its own hostname**, not just its own port. Cookies are keyed on
  host name and **ignore the port**, so two apps on `localhost:8080` and
  `localhost:8180` share one cookie jar: logging into one can silently replace the
  other's `XASSESSIONID`. Two hostnames give two jars, and the differing ports do no
  harm. Add them to `/etc/hosts` —

  ```
  127.0.0.1  backend.local frontend.local
  ```

  — and browse `http://backend.local:8080/` and `http://frontend.local:8180/`. The
  runtime binds `127.0.0.1` and serves any `Host` you send it, and the client uses
  relative URLs, so it works under any name that resolves to loopback. (`*.nip.io`
  works too if you would rather not touch `/etc/hosts`; prefer `/etc/hosts` in a
  locked-down container, where public wildcard DNS may not resolve — `localtest.me`
  resolves to `::1` in some of them.)

  Then record the name in each app's own configuration, so the runtime knows the URL
  it is reached at and generates absolute URLs — OIDC/SAML redirect URIs, deep links —
  against the host name rather than the listen address:

  ```sql
  alter settings configuration 'Default'
    ApplicationRootUrl = 'http://backend.local:8080/';
  ```

  `run --local` picks that up at boot and prints which configuration it came from.
  A blank app ships `http://localhost:8080/` there, and that stock loopback value is
  deliberately ignored — otherwise every project would start advertising a URL, and
  the wrong port under `--app-port`. Only a real host name is passed through.
- **Databases** need no action: the name is derived from the `.mpr` file name, so
  differently-named apps get different databases.
- **The session hook.** `mxcli init` writes `.claude/settings.json` inside each app
  folder, but Claude Code reads the one at the **repo root** — and it will not add a
  second entry for you (it dedupes on the command, not on the project). Write the root
  one yourself, one line per app, e.g.
  `test -x backend/mxcli && (cd backend && ./mxcli run --local --setup --ensure-db -p Backend.mpr) || true`.
  Verify it by checking that a fresh shell can boot each app.
- **Previews.** Pass `--hub-solution <SolutionName>` to every `run --hub` so the apps
  appear grouped in the hub overview instead of as unrelated previews.

**Wire the integration in dependency order — the producer must be running first.**
`CREATE ODATA CLIENT` fetches the `$metadata` at the moment you create it and caches
it in the model; if the URL is unreachable it warns and leaves the client unvalidated,
with no external entities to import. So: publish on the producer
(`CREATE ODATA SERVICE … publish entity …`), boot it (`run --local`), and only then,
on the consumer, `CREATE ODATA CLIENT … MetadataUrl: 'http://backend.local:8080/odata/…/$metadata'`
followed by `CREATE EXTERNAL ENTITIES FROM …`. Use the hostname here too, so the
cached contract and the constant below agree with what the browser sees. Point
`ServiceUrl` at a **constant** (`ServiceUrl: @Module.SvcUrl`) so the address can be
changed per environment without touching the model — it will not stay `localhost`.
`mxcli syntax odata.publish` and `mxcli syntax odata.consume` have the full syntax;
business events (`mxcli syntax business-events`) are the alternative when the link
should be asynchronous.

---

## Then propose the model — do not build it yet

The blank template ships a `MyFirstModule`; the app's own work belongs in a module
named after it. From the brief, propose in chat:

- a module name, and the entities from Q4 with their attributes and associations
- the user roles from Q5 and what each may read/write
- the handful of pages that make it usable
- for a solution: which app owns each entity, and what crosses the boundary — publish
  only what the other app actually needs

Show it as **MDL the user can read**, and wait for their go-ahead before executing it.
If a design was handed to you, it is the source of truth for the model and the pages —
see `migrate-design-prototype`.

---

## After bootstrap — the inner loop

```bash
./mxcli run --local -p <AppName>.mpr --watch --screenshot   # warm dev loop + screenshots
./mxcli exec change.mdl -p <AppName>.mpr                     # edit the model; the loop hot-applies
```

In a solution, run one loop per app from its own folder, with the second app on the
alternate ports, and start the producer first so the consumer's external entities
resolve:

```bash
(cd backend  && ./mxcli run --local -p Backend.mpr --watch)
(cd frontend && ./mxcli run --local -p Frontend.mpr --watch \
                  --app-port 8180 --admin-port 8190 --serve-port 6643)
```

See `run-local` for the warm loop, `--watch`, `--ensure-db`, and the screenshot
flags.
