#!/bin/bash
#
# PostToolUse hook (Bash) — after a `git push`, report where the branch stands
# relative to main.
#
# Why this exists: twice in one session commits were pushed onto a branch whose
# pull request had already been merged, and reported as "added to PR #N" when
# PR #N was closed and contained none of them. A merged PR cannot take new
# commits, so the work needed a fresh branch and a new PR.
#
# It would be better to name the PR directly, but this environment's egress
# proxy intercepts api.github.com and answers 403 ("GitHub access is not enabled
# for this session"), so a shell hook cannot ask. What it CAN compute locally is
# the state both failures shared: the branch was behind origin/main because main
# had absorbed the branch's earlier commits via the merge.
#
# Prints nothing when the branch is simply ahead of main — the normal case —
# so a clean push stays quiet.
set -uo pipefail

payload=$(cat)

# Only react to a push. Matched anywhere in the command rather than via the
# hook's `if` filter, which is a PREFIX match and would miss the common
# `git add … && git commit … && git push …`.
command=$(printf '%s' "$payload" | jq -r '.tool_input.command // ""' 2>/dev/null || true)
case "$command" in
  *"git push"*) ;;
  *) exit 0 ;;
esac

cd "${CLAUDE_PROJECT_DIR:-.}" 2>/dev/null || exit 0
git rev-parse --git-dir >/dev/null 2>&1 || exit 0

branch=$(git rev-parse --abbrev-ref HEAD 2>/dev/null || true)
if [ -z "$branch" ] || [ "$branch" = "HEAD" ]; then exit 0; fi

# Best effort: a slow or offline fetch must not hold up the session.
timeout 15 git fetch -q origin main >/dev/null 2>&1 || true
git rev-parse --verify -q origin/main >/dev/null 2>&1 || exit 0

behind=$(git rev-list --count "HEAD..origin/main" 2>/dev/null || echo 0)
if [ "$behind" -eq 0 ]; then exit 0; fi

ahead=$(git rev-list --count "origin/main..HEAD" 2>/dev/null || echo 0)

if [ "$ahead" -eq 0 ]; then
  # Nothing of its own: main has everything this branch has. Either its PR was
  # merged, or the checkout is stale (an ephemeral container is re-cloned fresh,
  # so locally-made commits can be absent while the pushed branch still has them).
  msg="$branch is $behind commit(s) behind origin/main and has none of its own.
Its pull request may already be merged, or this checkout may be stale — a fresh container
re-clones the repo, so commits made earlier in the session can be missing locally.
Check both before describing the branch's state:
  git log --oneline origin/$branch   (what was actually pushed)
  restart from main if the PR merged: git checkout -B $branch origin/main"
else
  msg="pushed $branch — it is $ahead commit(s) ahead of origin/main but also $behind BEHIND.
If a pull request from this branch was already merged, it CANNOT carry those $ahead commit(s):
verify the PR is still open before saying they were added to it.
If it merged, restart the branch and keep the unmerged work:
  git fetch origin main && git rebase --onto origin/main <merged-sha> $branch"
fi

# systemMessage surfaces it to the user; additionalContext puts the same fact in
# the model's context, which is where the wrong claim was made.
jq -nc --arg m "$msg" \
  '{systemMessage: $m, hookSpecificOutput: {hookEventName: "PostToolUse", additionalContext: $m}}'
