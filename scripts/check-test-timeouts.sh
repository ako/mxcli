#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
#
# Guard: every `go test` the Makefile runs must set its own -timeout.
#
# Why this exists — mendixlabs/mxcli#1291. `make test` ran `go test ./...` with no
# -timeout, so each test binary got Go's 10m default. mdl/linter passes in ~25s on
# its own, but under `./...` it shares the CPU with every other package and was
# measured at 552s on one contributor's machine and 872s on another's: the second
# reported `panic: test timed out after 10m0s` as a FAIL in a package nobody had
# touched. Whether it trips depends on machine load, so it looks like a flaky test
# rather than a missing flag. A limit nobody chose is the defect; this guard makes
# choosing one mandatory.
#
# Checks recipe lines (tab-indented, backslash continuations joined) that run
# `go test` directly or through a Makefile variable whose value runs it, such as
# INTEGRATION_GO_TEST. A variable whose own value sets -timeout satisfies its uses.
#
# Usage: scripts/check-test-timeouts.sh [Makefile]
set -uo pipefail

makefile="${1:-Makefile}"
if [ ! -f "$makefile" ]; then
  echo "check-test-timeouts: $makefile not found" >&2
  exit 2
fi

awk '
  # Join backslash continuations into one logical line.
  {
    line = $0
    while (line ~ /\\$/ && (getline next_line) > 0) {
      sub(/\\$/, "", line)
      line = line " " next_line
    }
    lines[++n] = line
    lineno[n] = NR
  }
  END {
    # Pass 1: variables whose value runs go test without its own -timeout.
    for (i = 1; i <= n; i++) {
      l = lines[i]
      if (l ~ /^\t/) continue
      if (match(l, /^[A-Za-z_][A-Za-z0-9_]*[ \t]*[:?+]?=/)) {
        name = l
        sub(/[ \t]*[:?+]?=.*/, "", name)
        if (l ~ /go test/ && l !~ /-timeout/) runs_go_test[name] = 1
      }
    }

    target = ""
    for (i = 1; i <= n; i++) {
      l = lines[i]
      if (l !~ /^\t/) {
        if (match(l, /^[A-Za-z0-9_.-]+:/) && l !~ /^[A-Za-z_][A-Za-z0-9_]*[ \t]*[:?+]?=/) {
          target = substr(l, 1, RLENGTH - 1)
        }
        continue
      }
      runs = (l ~ /go test/)
      for (v in runs_go_test) {
        if (index(l, "$(" v ")") || index(l, "${" v "}")) runs = 1
      }
      if (!runs) continue
      checked++
      if (target == "test") saw_test_target = 1
      if (l !~ /-timeout[ =]/) {
        cmd = l
        sub(/^\t[@-]*/, "", cmd)
        printf "%s:%d: target %s runs go test without -timeout:\n    %s\n", FILENAME, lineno[i], target, cmd
        bad++
      }
    }

    # Positive control: the guard must actually see `make test`, or a change to
    # how that target is written would make it pass vacuously.
    if (!saw_test_target) {
      printf "check-test-timeouts: found no go test in the `test` target of %s; the guard cannot see what it guards\n", FILENAME
      exit 1
    }
    if (bad) {
      printf "\n%d go test invocation(s) with no -timeout. Go then applies 10m per test\n", bad
      printf "binary, and a package that is fast alone can exceed it under load (#1291).\n"
      printf "Set an explicit limit with headroom over the slowest measured run.\n"
      exit 1
    }
    printf "check-test-timeouts: %d go test invocation(s), all with -timeout\n", checked
  }
' "$makefile"
