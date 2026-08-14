#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
#
# Guard: the embedded tunnel (chisel and its tunnelling-specific dependencies)
# must not reach the Windows or macOS builds.
#
# Why this exists — see docs/13-decisions/0009-tunnel-is-linux-only.md. Chisel is
# a dual-use pivoting tool that appears in threat intelligence; shipping it in
# binaries that never use it gets mxcli flagged by Defender and enterprise EDR,
# which blocks adoption on managed corporate endpoints. The tunnel only ever runs
# inside a Linux container, so it is built only for Linux.
#
# Without this guard the import comes back the next time someone edits the hub
# code, and nothing would notice until a user's EDR does.
#
# Usage:
#   scripts/check-tunnel-deps.sh              # dependency-graph check (default)
#   scripts/check-tunnel-deps.sh --binary P   # also inspect a built binary at P
set -uo pipefail

# Modules that must never appear in a non-Linux dependency graph. This is a
# module list, not a grep for "chisel": the SSH/websocket/socks stack is most of
# what makes an EDR classifier fire, and it would come back through a transitive
# edge without the name "chisel" appearing anywhere.
FORBIDDEN=(
  "github.com/jpillora/chisel"
  "github.com/jpillora/ansi"
  "github.com/jpillora/backoff"
  "github.com/jpillora/requestlog"
  "github.com/jpillora/sizestr"
  "github.com/gorilla/websocket"
  "github.com/armon/go-socks5"
  "github.com/andrew-d/go-termutil"
  "github.com/tomasen/realip"
  "golang.org/x/crypto/ssh"
  "golang.org/x/net/proxy"
)

fail=0

# --- 1. Positive control -----------------------------------------------------
# Prove the check can actually see chisel before trusting it to report absence.
# Without this, a typo'd package pattern or a `go list` that errors out would
# make every platform look clean and the guard would pass vacuously.
linux_hits=$(GOOS=linux GOARCH=amd64 go list -deps ./... 2>/dev/null | grep -c '^github.com/jpillora/chisel')
if [ "$linux_hits" -eq 0 ]; then
  echo "FAIL: positive control — expected chisel in the linux dependency graph, found none."
  echo "      Either the tunnel was removed entirely (update this guard) or 'go list' is broken."
  echo "      Refusing to report the other platforms clean on the strength of a check that sees nothing."
  exit 1
fi
echo "ok: positive control — linux graph contains chisel ($linux_hits packages)"

# --- 2. The guard ------------------------------------------------------------
for goos in windows darwin; do
  for goarch in amd64 arm64; do
    deps=$(GOOS="$goos" GOARCH="$goarch" go list -deps ./... 2>/dev/null)
    if [ -z "$deps" ]; then
      echo "FAIL: could not compute the $goos/$goarch dependency graph."
      fail=1
      continue
    fi
    hits=""
    for mod in "${FORBIDDEN[@]}"; do
      # Match the module path or any package under it, not a substring.
      found=$(printf '%s\n' "$deps" | grep -E "^${mod}(/|$)" || true)
      [ -n "$found" ] && hits+="$found"$'\n'
    done
    if [ -n "$hits" ]; then
      echo "FAIL: $goos/$goarch links tunnel code that must be Linux-only:"
      printf '%s' "$hits" | sed 's/^/        /'
      fail=1
    else
      echo "ok: $goos/$goarch is free of tunnel dependencies"
    fi
  done
done

# --- 3. Optional binary inspection ------------------------------------------
# The dependency graph is the authoritative check; this confirms it against a
# real artifact. Note that `go tool nm` is useless on a release binary: the
# release ldflags (-s -w) strip the symbol table, so nm reports "no symbols"
# whether or not chisel is linked. Module info and string literals survive.
if [ "${1:-}" = "--binary" ] && [ -n "${2:-}" ]; then
  bin="$2"
  echo "--- inspecting $bin ---"
  if go version -m "$bin" 2>/dev/null | grep -Eq "$(IFS='|'; echo "${FORBIDDEN[*]}")"; then
    echo "FAIL: build info in $bin still lists a forbidden module:"
    go version -m "$bin" | grep -E "$(IFS='|'; echo "${FORBIDDEN[*]}")" | sed 's/^/        /'
    fail=1
  else
    echo "ok: no forbidden module in the binary's build info"
  fi
  n=$(strings -n 6 "$bin" 2>/dev/null | grep -ci chisel || true)
  if [ "${n:-0}" -ne 0 ]; then
    echo "FAIL: $n chisel string literals found in $bin"
    fail=1
  else
    echo "ok: no chisel string literals in the binary"
  fi
fi

if [ "$fail" -ne 0 ]; then
  echo
  echo "The tunnel must stay behind the Linux-only seam:"
  echo "  cmd/mxcli/docker/tunnel_linux.go      (chisel client)"
  echo "  cmd/mxcli/tunnelhub/control_linux.go  (chisel server)"
  echo "with a !linux stub beside each. See docs/13-decisions/0009-tunnel-is-linux-only.md."
  exit 1
fi
echo "All platforms clean."
