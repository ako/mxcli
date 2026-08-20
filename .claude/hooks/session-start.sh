#!/bin/bash
#
# SessionStart hook — bootstrap a Claude Code on the web session for mxcli.
#
# The devcontainer (.devcontainer/Dockerfile) installs these prerequisites, but
# web sessions do not use the devcontainer, so a fresh container has no ANTLR4
# and `make build` fails at the `make grammar` step:
#
#     *** ANTLR4 not found. Install with: brew install antlr4 ... Stop.
#
# The generated parser in mdl/grammar/parser/ is deliberately not committed, so
# ANTLR4 is a hard build dependency, not an optional extra.
#
# Idempotent and non-interactive: safe to re-run on resume/clear/compact.
set -euo pipefail

# Pinned to match .github/workflows/push-test.yml. The antlr4 wrapper resolves
# the jar version from this variable, so it must be set for every build, not
# just this script — hence the CLAUDE_ENV_FILE export below.
ANTLR_VERSION='4.13.2'
ANTLR_TOOLS_VERSION='0.2.2'

cd "${CLAUDE_PROJECT_DIR:-$(dirname "$0")/../..}" 2>/dev/null || exit 0

# 0. Is this checkout what the session thinks it is?
#
# The container is ephemeral: it is re-cloned when reprovisioned, so commits made
# earlier in a session can be absent from the working copy while still present on
# the remote. That happened twice in one session, and the second time it was
# misread as a code bug — a grammar rule "missing" from the tree had in fact been
# committed and pushed hours earlier, and the binary under test was built from
# the rolled-back tree.
#
# Only speaks when the branch is actually behind its own remote, so a healthy
# start stays silent. Runs before the remote-only exit below because a stale
# checkout is worth knowing about on any machine.
_branch=$(git rev-parse --abbrev-ref HEAD 2>/dev/null || true)
if [ -n "${_branch:-}" ] && [ "${_branch}" != "HEAD" ]; then
  timeout 15 git fetch -q origin "${_branch}" >/dev/null 2>&1 || true
  if git rev-parse --verify -q "origin/${_branch}" >/dev/null 2>&1; then
    _behind=$(git rev-list --count "HEAD..origin/${_branch}" 2>/dev/null || echo 0)
    if [ "${_behind}" -gt 0 ]; then
      echo "WARNING: HEAD is ${_behind} commit(s) behind origin/${_branch}."
      echo "  This checkout may be a fresh clone that is missing work pushed earlier."
      echo "  Reconcile before building or testing:  git log --oneline origin/${_branch}"
    fi
  fi
fi

# The binary carries the commit it was built from (Makefile -X main.Version), so
# a mismatch means any behaviour observed through bin/mxcli is from other code.
if [ -x bin/mxcli ]; then
  _built=$(bin/mxcli --version 2>/dev/null | grep -oE '[0-9a-f]{7,}' | head -1 || true)
  _head=$(git rev-parse --short HEAD 2>/dev/null || true)
  if [ -n "${_built:-}" ] && [ -n "${_head:-}" ] && [ "${_built}" != "${_head}" ]; then
    echo "NOTE: bin/mxcli was built from ${_built}, HEAD is ${_head} — run 'make build' before testing."
  fi
fi

# Local (devcontainer / laptop) setups already have these via the Dockerfile.
if [ "${CLAUDE_CODE_REMOTE:-}" != "true" ]; then
  exit 0
fi

# 1. ANTLR4 — required by `make grammar`, which `make build` always runs.
if ! command -v antlr4 >/dev/null 2>&1; then
  echo "Installing antlr4-tools==${ANTLR_TOOLS_VERSION}..."
  pip install --break-system-packages --quiet "antlr4-tools==${ANTLR_TOOLS_VERSION}"
fi

# The antlr4 wrapper downloads its jar on first use. Doing it here keeps that
# ~2MB fetch (and its JDK probe) out of the first `make build`.
export ANTLR4_TOOLS_ANTLR_VERSION="${ANTLR_VERSION}"
if [ ! -d "${HOME}/.m2/repository/org/antlr/antlr4/${ANTLR_VERSION}" ]; then
  echo "Fetching ANTLR ${ANTLR_VERSION} jar..."
  antlr4 >/dev/null 2>&1 || true
fi

# Persist for the session so `make build` works from any later shell.
if [ -n "${CLAUDE_ENV_FILE:-}" ]; then
  echo "export ANTLR4_TOOLS_ANTLR_VERSION=${ANTLR_VERSION}" >> "${CLAUDE_ENV_FILE}"
fi

# 2. Go modules — warms the module cache so the first build is not a ~1GB fetch.
echo "Downloading Go modules..."
go mod download

# 3. MxBuild — the Mendix toolchain that validates projects mxcli writes
#    (`mx check`). Opt-in: the CDN tarball is ~820MB and unpacks to ~1.6GB, too
#    slow to pull into every session start. Set the version to enable, e.g.
#    MXCLI_HOOK_MXBUILD_VERSION=11.13.0 (the newest in the nightly CI matrix).
#    Otherwise fetch on demand: mxcli setup mxbuild --version 11.13.0
if [ -n "${MXCLI_HOOK_MXBUILD_VERSION:-}" ]; then
  if [ ! -d "${HOME}/.mxcli/mxbuild/${MXCLI_HOOK_MXBUILD_VERSION}" ]; then
    echo "Downloading MxBuild ${MXCLI_HOOK_MXBUILD_VERSION} (~820MB)..."
    go run ./cmd/mxcli setup mxbuild --version "${MXCLI_HOOK_MXBUILD_VERSION}"
  fi
fi

echo "Session bootstrap complete. Build with: make build"
