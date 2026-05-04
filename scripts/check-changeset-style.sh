#!/usr/bin/env bash
# Lint changeset bodies for typography characters that should be ASCII
# in the auto-generated CHANGELOG.
#
# Why scoped tightly: changesets and CHANGELOG entries are auto-published
# verbatim by `monorel release`. Prose-quality regressions in those
# files ship with the next tag and can't be edited out without
# re-cutting a release. Other prose (docs/src, README) gets a normal
# review cycle; this script intentionally does not police it.
#
# Scoping note: the script scans every `.changeset/*.md` on disk, not
# just the ones currently staged. Lefthook's `glob:` controls when the
# step fires, but once it fires the script checks all of them. A
# latent typography character in an older changeset still ships into
# CHANGELOG at the next release, so it's worth catching on any commit
# that touches the directory.
#
# Run locally:
#   bash scripts/check-changeset-style.sh
#
# Self-test:
#   bash scripts/test-check-changeset-style.sh
#
# Wired into:
#   - lefthook pre-commit (when .changeset/*.md changes)
#   - CI .github/workflows/ci.yml (every push and PR)

set -euo pipefail

# Banned typography characters, each with the ASCII replacement the
# project's docs rule prescribes. Keep this list synchronized with
# .claude/rules/documentation.md.
#
# Format: "<utf-8 byte sequence>|<U+ codepoint>|<name>|<replacement hint>"
# The codepoint and name are for the error message; the replacement
# hint helps the contributor know what to type instead.
BANNED=(
  $'\xe2\x80\x94|U+2014|EM DASH|use a colon, period, semicolon, or parens (see .claude/rules/documentation.md)'
  $'\xe2\x80\x93|U+2013|EN DASH|use a hyphen for ranges (1-10), or "to" prose ("1 to 10")'
  $'\xe2\x80\x98|U+2018|LEFT SINGLE QUOTATION MARK|use ASCII apostrophe (\x27)'
  $'\xe2\x80\x99|U+2019|RIGHT SINGLE QUOTATION MARK|use ASCII apostrophe (\x27)'
  $'\xe2\x80\x9c|U+201C|LEFT DOUBLE QUOTATION MARK|use ASCII double quote (")'
  $'\xe2\x80\x9d|U+201D|RIGHT DOUBLE QUOTATION MARK|use ASCII double quote (")'
  $'\xe2\x80\xa6|U+2026|HORIZONTAL ELLIPSIS|use three ASCII dots (...)'
)

# Collect target files. Empty match (no pending changesets) → exit 0.
TARGETS=()
shopt -s nullglob
for f in "${CHANGESET_DIR:-.changeset}"/*.md; do
  case "$(basename "$f")" in
    README.md) continue ;;
  esac
  TARGETS+=("$f")
done
shopt -u nullglob

if [ ${#TARGETS[@]} -eq 0 ]; then
  exit 0
fi

violations=0
for entry in "${BANNED[@]}"; do
  IFS='|' read -r char codepoint name fix <<< "$entry"
  for f in "${TARGETS[@]}"; do
    if grep -nF "$char" "$f" >/dev/null 2>&1; then
      while IFS= read -r line; do
        echo "::error file=$f::$codepoint $name: $fix" >&2
        echo "  $line" >&2
      done < <(grep -nF "$char" "$f")
      violations=$((violations + 1))
    fi
  done
done

if [ "$violations" -gt 0 ]; then
  echo "" >&2
  echo "$violations changeset typography violation(s) found. Replace with ASCII and re-stage." >&2
  exit 1
fi
