---
title: The Warm Loop Fails Quietly
category: bug-pattern
last-synced: 5c24d899
covers:
  - cmd/mxcli
sources:
  - .claude/skills/fix-issue/findings/cmd-mxcli/
  - cmd/mxcli/docker/localboot.go
  - cmd/mxcli/docker/runlocal.go
  - cmd/mxcli/docker/webclient_supervisor.go
  - cmd/mxcli/docker/webclient_watch.go
  - cmd/mxcli/docker/webclient_pages.go
  - cmd/mxcli/devloop_admin.go
---

> **Do not duplicate**: the flags and their behaviour live in
> `.claude/skills/mendix/run-local.md`; the individual fixes live in the
> findings. This page describes the shape of failure in a loop mxcli only partly
> owns.

## What this is

`mxcli run --local` orchestrates processes it did not write: mxbuild, a JVM
runtime, a rollup bundler, PostgreSQL, and a browser. Sixteen `cmd/mxcli`
findings are failures in that loop, and they share a signature — **the thing that
breaks is not the thing that reports**. A page goes black with HTTP 200. The
runtime is gone and the CLI sits at several hundred percent CPU. Styles do not
change and nothing is stale. A login that is correct reports "Sign in failed".

## How it fits

**A missing asset answers 200.** Mendix serves an SPA shell before it knows
whether the client bundle exists, so a deleted `deployment/web/dist` renders a
black screen while every HTTP-level check passes. `curl /` is not a liveness
probe for this app; the bundle is.

**A dead child does not stop the parent.** A runtime that exited left a zombie
under a CLI that kept polling, for hours. Anything that supervises a long-lived
child needs to reap it and exit, or the failure presents as a hang rather than an
error.

**The log the user needs is not the log being captured.** Several rounds went
into `runtime.log` holding only the JVM banner: the application's own logging
goes through a subscriber that attaches after startup, so anything logged
*during* the start action — which is exactly where the after-startup test runner
logged — is not in the file. Capturing "the process's stdout" and capturing "the
app's log" are different jobs.

**Everything the loop resolves for another tool is a place to disagree
silently.** A relative `-p` reaches mxbuild as its own error text plus a page of
Windows JSON. A version resolved for one component and defaulted in another
produces a project at a Mendix version nobody asked for. An architecture picked
for `docker build` and not for `--local` gives `exec format error`. A credential
that is right for one API is wrong for the other — the M2EE admin password and
the test endpoint token are different secrets, and passing one where the other
belongs reports "Authentication failed" after the model has already been
modified.

**Guards that name a cause can name the wrong one.** A port-in-use guard blamed a
previous `run --local` that the user had already killed. A guard is a diagnosis,
and a wrong diagnosis costs more than a plain error — this is why the port owner
is now resolved from `/proc` rather than asserted.

**Stale state has more than one home.** A theme edit that does not appear, a
constant that does not reach a running app, a page that dies after ten
hot-applies with a 404 for a hashed chunk — each is a cache or a running process
holding something the file no longer says. The loop's value is that it does not
restart; the cost is that "what is actually being served" and "what is on disk"
are separate questions, and the answer has to be printed rather than assumed.

**A watcher's baseline is a source time, not a clock reading.** This one has
shipped twice, in both directions. The loop took `last = sourceMTime(...)` *after*
each successful apply and again after the boot finished — so any edit made during
a build or during the boot was folded into the baseline and never detected: no
`Change detected`, the app serving the previous model, and re-running the script
writing nothing because the write was already idempotent. The baseline has to be
the source time the last build was **made from**, captured before that build. The
test that catches it makes an edit *during* a build, not between builds, which is
the only arrangement either instance was visible in.

**A supervised child is a supervisor's job, not a handle.** The loop held a bare
pointer to the web-client bundler, so when the bundler died nothing restarted it
and every later change failed; a failed incremental build left it in a state no
later change could leave. The general remedy is that **a fresh bundler is the
universal recovery**: its first build is a full bundle of the current source, so
it covers a dead watcher, missing pages, dangling chunks and a `dist/` someone
else deleted. Reproducing it needs no patience — kill the runner process and the
dead-watcher state is there in seconds.

**The tools the loop drives have their own idea of what is being watched.**
mxbuild's page plugin globs `web/pages` once, at bundler start, and matches later
events against that snapshot — so a page added while the loop runs is never
bundled, and the apply still reports success. A stand-alone repro of the third
party's watcher (add a second file, watch the next bundle emit only the first)
isolates this in seconds and keeps the diagnosis out of mxcli's code. Two
consequences pull against each other: the bundler's working directory cannot be
moved out of `web/` to dodge a Windows directory lock, because the page plugin
resolves paths relative to that cwd.

**Whoever knows the port must publish it, and the consumers must read it.** The
loop prints hints — "Query data: `mxcli oql -p app.mpr`" — that failed on a
non-default admin port, because `oql` and `log` each resolved the port from
flags, env and `.docker/.env` and never read the handshake the loop had just
written. A hint printed by the process that knows a value has to be runnable by a
reader that does not; reading the published handshake fixes every consumer at
once, where adding the flag to the hint fixes one line.

**A flag that consumes another flag's setup silently degrades alone.**
`--page-check` reuses the Playwright storage state that the `--screenshot-user`
step writes, and the login step was gated on the screenshot flags — so every
secured page was checked as the sign-in form and reported healthy, because a
login page *is* a healthy page. The tell was the title. Where one flag depends on
another's side effect, the dependency belongs in the gate, and the checker itself
should fail on the sign-in form so the same mistake cannot pass twice.

**Two boot paths mean two sets of defaults.** `run --local` and `test --local`
start the app through different functions, each owning its own database
configuration, so `--db-type` existed on one and not the other and a machine
without PostgreSQL could run the app but not the tests. The same shape put two
mxbuild resolvers in one command, where the result of the correct one was passed
only to the bundler and the serve step defaulted. When adding a capability to one
boot path, grep for the other.

**Half-applied is a state the loop can reach.** A watch that redeploys while an
`mxcli exec` is mid-script can serve a partial model, and re-running the script
fixes nothing because the write is idempotent and there is nothing left to write.

## See also

- [fix-issue findings](../../.claude/skills/fix-issue/findings/) — the individual
  probes, guards and controls
- `.claude/skills/mendix/run-local.md` — what the flags actually do
- `.claude/skills/verify-in-runtime.md` — proving a fix in a real browser, which
  is the only check that sees most of this class
- [[the-platform-ci-does-not-run]] — the environments several of these failures
  live in exclusively
