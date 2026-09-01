---
title: The Warm Loop Fails Quietly
category: bug-pattern
last-synced: ced830e0
sources:
  - .claude/skills/fix-issue/findings/cmd-mxcli.jsonl
  - cmd/mxcli/docker/localboot.go
  - cmd/mxcli/docker/runlocal.go
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

**Half-applied is a state the loop can reach.** A watch that redeploys while an
`mxcli exec` is mid-script can serve a partial model, and re-running the script
fixes nothing because the write is idempotent and there is nothing left to write.

## See also

- [fix-issue findings](../../.claude/skills/fix-issue/findings/) — the individual
  probes, guards and controls
- `.claude/skills/mendix/run-local.md` — what the flags actually do
- `.claude/skills/verify-in-runtime.md` — proving a fix in a real browser, which
  is the only check that sees most of this class
