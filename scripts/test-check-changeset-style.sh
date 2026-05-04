#!/usr/bin/env bash
# Self-test for scripts/check-changeset-style.sh.
#
# Drives the lint against fixture inputs and asserts each expected
# violation is reported (or not) per the typography rule.
#
# Run locally:
#   bash scripts/test-check-changeset-style.sh
#
# Wired into:
#   - .github/workflows/ci.yml (every push and PR)

set -euo pipefail

LINT="$(cd "$(dirname "$0")" && pwd)/check-changeset-style.sh"
if [ ! -x "$LINT" ]; then
  echo "lint script not executable: $LINT" >&2
  exit 1
fi

# Each test runs the lint against a fixture directory and asserts the
# exit code + (optionally) that a substring appears in stderr.
pass_count=0
fail_count=0

# Run the lint against a fixture directory. Captures stderr for the
# substring check. Stdout is the lint's "OK" line on pass.
run_case() {
  local name="$1" want_exit="$2" want_substr="${3:-}" fixture_dir="$4"
  local stderr
  set +e
  stderr=$(CHANGESET_DIR="$fixture_dir" bash "$LINT" 2>&1 >/dev/null)
  local got_exit=$?
  set -e
  if [ "$got_exit" != "$want_exit" ]; then
    echo "FAIL [$name]: exit=$got_exit, want $want_exit" >&2
    echo "  stderr: $stderr" >&2
    fail_count=$((fail_count + 1))
    return
  fi
  if [ -n "$want_substr" ] && ! echo "$stderr" | grep -qF "$want_substr"; then
    echo "FAIL [$name]: stderr missing substring: $want_substr" >&2
    echo "  stderr: $stderr" >&2
    fail_count=$((fail_count + 1))
    return
  fi
  echo "PASS [$name]"
  pass_count=$((pass_count + 1))
}

# Build all fixture directories under one tmpdir; cleaned up on exit.
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

# Case 1: empty .changeset/ directory -> exit 0 (no-op).
mkdir "$TMP/empty"
run_case "empty-directory" 0 "" "$TMP/empty"

# Case 2: only a README.md (the conventional non-changeset marker) -> exit 0.
mkdir "$TMP/only-readme"
echo "# Changesets" > "$TMP/only-readme/README.md"
run_case "only-readme" 0 "" "$TMP/only-readme"

# Case 3: clean changeset with allowed ASCII typography -> exit 0.
mkdir "$TMP/clean"
cat > "$TMP/clean/feat.md" <<'EOF'
---
"pkg-a": minor
---

Add support for X. The behavior matches Y; see issue (link) for context.

Use ASCII 'quotes', "double quotes", and three dots (...) only.
EOF
run_case "clean-changeset" 0 "" "$TMP/clean"

# Case 4: em-dash -> exit 1, error mentions U+2014.
mkdir "$TMP/em-dash"
printf 'fix em \xe2\x80\x94 dash here\n' > "$TMP/em-dash/bad.md"
run_case "em-dash" 1 "U+2014 EM DASH" "$TMP/em-dash"

# Case 5: en-dash -> exit 1, error mentions U+2013.
mkdir "$TMP/en-dash"
printf 'numeric range 1\xe2\x80\x9310 here\n' > "$TMP/en-dash/bad.md"
run_case "en-dash" 1 "U+2013 EN DASH" "$TMP/en-dash"

# Case 6: smart single quotes -> exit 1, both directions reported.
mkdir "$TMP/smart-single"
printf 'apostrophe \xe2\x80\x98smart\xe2\x80\x99 quotes\n' > "$TMP/smart-single/bad.md"
run_case "smart-single-left" 1 "U+2018 LEFT SINGLE QUOTATION MARK" "$TMP/smart-single"
run_case "smart-single-right" 1 "U+2019 RIGHT SINGLE QUOTATION MARK" "$TMP/smart-single"

# Case 7: smart double quotes -> exit 1, both directions reported.
mkdir "$TMP/smart-double"
printf 'curly \xe2\x80\x9cdoubles\xe2\x80\x9d here\n' > "$TMP/smart-double/bad.md"
run_case "smart-double-left" 1 "U+201C LEFT DOUBLE QUOTATION MARK" "$TMP/smart-double"
run_case "smart-double-right" 1 "U+201D RIGHT DOUBLE QUOTATION MARK" "$TMP/smart-double"

# Case 8: ellipsis -> exit 1, error mentions U+2026.
mkdir "$TMP/ellipsis"
printf 'something missing\xe2\x80\xa6 here\n' > "$TMP/ellipsis/bad.md"
run_case "ellipsis" 1 "U+2026 HORIZONTAL ELLIPSIS" "$TMP/ellipsis"

# Case 9: multiple violations across multiple files -> exit 1, count
# message reports plural.
mkdir "$TMP/multi"
printf 'em \xe2\x80\x94 dash\n' > "$TMP/multi/a.md"
printf 'curly \xe2\x80\x9cdouble\xe2\x80\x9d\n' > "$TMP/multi/b.md"
run_case "multi-file" 1 "violation(s) found" "$TMP/multi"

# Case 10: README.md with em-dash is skipped (the script excludes README).
mkdir "$TMP/readme-em"
printf 'em \xe2\x80\x94 dash in README\n' > "$TMP/readme-em/README.md"
run_case "readme-skipped" 0 "" "$TMP/readme-em"

# Case 11: a clean changeset alongside a violating one -> exit 1
# (any single violation fails the run).
mkdir "$TMP/mixed"
cat > "$TMP/mixed/clean.md" <<'EOF'
fix: clean ASCII only.
EOF
printf 'em \xe2\x80\x94 dash\n' > "$TMP/mixed/bad.md"
run_case "mixed-clean-and-bad" 1 "U+2014 EM DASH" "$TMP/mixed"

echo ""
total=$((pass_count + fail_count))
echo "ran $total case(s); $pass_count passed, $fail_count failed"
if [ "$fail_count" -gt 0 ]; then
  exit 1
fi
