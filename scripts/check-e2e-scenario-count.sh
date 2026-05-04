#!/usr/bin/env bash
# Verify that tests/e2e/README.md's "N scenarios in M files" claim
# matches the actual counts in tests/e2e/.
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

# Count top-level Test* functions (`func TestX(t *testing.T) {`).
# TestMain has signature `(m *testing.M)`, so this regex naturally
# excludes it. Subtests via `t.Run` aren't counted; the README
# convention is "1 Test func = 1 scenario" regardless of how many
# sub-cases it sweeps.
actual_scenarios=$(grep -hE '^func Test[A-Za-z_]+\(t \*testing\.T\)' tests/e2e/*_test.go | wc -l)

# Count scenario-bearing files. Excludes helpers_test.go and
# main_test.go (test-runner plumbing, no scenarios) so the count
# matches what's listed in the README's per-file coverage table.
actual_files=$(ls tests/e2e/*_test.go \
  | grep -vE '/(helpers_test|main_test)\.go$' \
  | wc -l)

# Pull the asserted counts from the README. The line shape is:
#   `<N> scenarios in <M> files. ...`
# (See tests/e2e/README.md.)
line=$(grep -oE '^[0-9]+ scenarios in [0-9]+ files' tests/e2e/README.md | head -1)
if [ -z "$line" ]; then
  echo "::error file=tests/e2e/README.md::could not find 'N scenarios in M files' line" >&2
  exit 1
fi
claimed_scenarios=$(echo "$line" | awk '{print $1}')
claimed_files=$(echo "$line" | awk '{print $4}')

drift=0
if [ "$actual_scenarios" != "$claimed_scenarios" ]; then
  echo "::error file=tests/e2e/README.md::e2e scenario count drift: README claims $claimed_scenarios, actual is $actual_scenarios" >&2
  drift=1
fi
if [ "$actual_files" != "$claimed_files" ]; then
  echo "::error file=tests/e2e/README.md::e2e file count drift: README claims $claimed_files files, actual is $actual_files" >&2
  drift=1
fi

if [ "$drift" -ne 0 ]; then
  echo "" >&2
  echo "Update tests/e2e/README.md's '$claimed_scenarios scenarios in $claimed_files files.' line and the" >&2
  echo "per-file coverage table to match. Then re-run this script." >&2
  exit 1
fi

echo "OK: tests/e2e/ has $actual_scenarios scenarios in $actual_files files; README matches."
