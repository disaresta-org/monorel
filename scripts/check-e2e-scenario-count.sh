#!/usr/bin/env bash
# Verify that tests/e2e/README.md's "N scenarios in M files" claim
# matches the actual count of `Test*` functions in tests/e2e/.
#
# Why: the README's coverage table is hand-maintained. A new scenario
# without a matching README update silently drifts; this catches it
# before a reviewer has to.
#
# Run locally:
#   bash scripts/check-e2e-scenario-count.sh
#
# Wired into:
#   - .github/workflows/ci.yml (every push and PR)

set -euo pipefail

# Count top-level Test* functions (`func TestX(t *testing.T) {`),
# excluding TestMain (test-runner setup, not a scenario). Subtests via
# `t.Run` aren't counted; the README convention is "1 Test func = 1
# scenario" regardless of how many sub-cases it sweeps.
actual=$(grep -hE '^func Test[A-Za-z_]+\(t \*testing\.T\)' tests/e2e/*_test.go \
  | grep -v '^func TestMain' \
  | wc -l)

# Pull the asserted scenario count from the README. The line shape is:
#   `<N> scenarios in <M> files. ...`
# (See tests/e2e/README.md.)
claimed=$(grep -oE '^[0-9]+ scenarios in [0-9]+ files' tests/e2e/README.md \
  | head -1 | awk '{print $1}')

if [ -z "$claimed" ]; then
  echo "::error file=tests/e2e/README.md::could not find 'N scenarios in M files' line" >&2
  exit 1
fi

if [ "$actual" != "$claimed" ]; then
  echo "::error file=tests/e2e/README.md::e2e scenario count drift: README claims $claimed, actual is $actual" >&2
  echo "" >&2
  echo "Update tests/e2e/README.md's '$claimed scenarios in N files.' line and the" >&2
  echo "per-file coverage table to match. Then re-run this script." >&2
  exit 1
fi

echo "OK: tests/e2e/ has $actual scenarios; README matches."
