---
description: Diagnose and fix a reported bug, with the evidence bar the PR checklist expects
---

# /mxcli-dev:fix-issue — Fix a Reported Bug

Work a bug report end to end: match it against what has already been seen, fix it,
and prove the fix with evidence a reviewer can check.

Read [`.claude/skills/fix-issue.md`](../../skills/fix-issue.md) first — it holds the
findings-lookup mechanics and the two rules that are easiest to skip. This command
is the order to do things in.

`$ARGUMENTS` is the issue number or a description of the symptom.

## Steps

1. **Read the issue in full**, including comments. Quote the reported symptom
   verbatim somewhere — it is what the test has to reproduce, and paraphrasing it
   is how a fix ends up addressing a different bug.

2. **Match the failure class**, then the instance:
   - `docs-wiki/bug-patterns/` for the class (small, read it)
   - `grep -il '<CE code or keyword>' .claude/skills/fix-issue/findings/*.jsonl`
     for the instance

   A pattern-page miss means "not yet digested", never "not seen before".

3. **Establish the version facts before theorising**, if the report involves one.
   The arbiter for a metamodel property is the Mendix Model SDK's own
   `StructureVersionInfo` (`npm pack mendixmodelsdk`, then grep `src/gen/*.js` for
   the type) — not release notes, and not a number already written down in this
   repo. mendixlabs/mxcli#1121 was a floor copied from a proposal's illustrative
   sample output that nobody had ever measured.

4. **Write the failing test first**, at the layer the symptom lives in. Table of
   layer → package is in the skill.

5. **Prove the test detects the bug.** Revert the fix, or stub the guard, and
   confirm it fails *with the reported symptom*. Put the control's output in the PR.
   A test that has only ever run against fixed code has not been shown to detect
   anything.

6. **Run the real thing when the argument calls for it.** Required when the fix's
   justification asserts what a Mendix tool accepts or rejects — a version guard, a
   refusal, anything resting on "mxbuild would catch this". Also when the symptom is
   a property of the running app (`verify-in-runtime.md`). Cheapest form:

   ```bash
   mxcli new <Name> --version <full version> --theme none --layout none --skip-init
   mxcli exec <repro>.mdl -p <Name>.mpr
   mxcli docker check -p <Name>.mpr          # and again with the fault forced back in
   mxcli run --local -p <Name>.mpr           # when it has to render
   ```

   Build **two** copies — fixed, and with the fault forced back in — or the run
   tells you nothing you did not already believe.

7. **Add the regression case**: `mdl-examples/bug-tests/<issue>-<description>.mdl`,
   and check it parses (`mxcli check`) and passes `make check-mdl`.

8. **Append one finding** to `.claude/skills/fix-issue/findings/<area>.jsonl` and run
   `make check-findings`. Write the insight — what would have made this cheaper to
   find, and which plausible wrong turn to skip — not the changelog.

9. `make build && make test && make lint`, then commit.

## Before you say it cannot be verified

That claim ends the investigation, so it needs the evidence a fix would. State what
a positive result would look like; try the case you are sure works as a control; try
a different *shape* of query rather than another value. Then, if it still holds, say
it with the evidence attached rather than as a property of the environment.

On #1121 the claim was "Mendix 10 is not downloadable here", from a dozen uniform
404s with 11.x succeeding on the same host. It was wrong — Mendix 9 and 10 publish
four-part names with a build number — and a uniform negative across a whole class
was the tell that the query, not the class, was at fault.

## Done when

- [ ] Reported symptom reproduced by a test before the fix existed
- [ ] Control run recorded (reverted fix → test fails with that symptom)
- [ ] Full run done, with both variants, if the argument asserts what a tool accepts
- [ ] Any "cannot verify" claim carries its evidence and a falsifying control
- [ ] Bug-test MDL committed; `make check-mdl` passes
- [ ] Finding appended; `make check-findings` passes
- [ ] `make build && make test && make lint` pass
