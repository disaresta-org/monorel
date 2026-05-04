#!/usr/bin/env bash
# Lint changeset bodies (and other always-released prose) for the
# project's documented style rules.
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
# latent em-dash in an older changeset still ships into CHANGELOG at
# the next release, so it's worth catching on any commit that touches
# the directory.
#
# Run locally:
#   bash scripts/check-changeset-style.sh
#
# Wired into:
#   - lefthook pre-commit (when .changeset/*.md changes)
#   - CI .github/workflows/ci.yml (every push and PR)

set -euo pipefail

# Files in scope. Glob expansion happens in the loop below; an empty
# match (e.g. no pending changesets) is fine and treated as a no-op.
TARGETS=()
shopt -s nullglob
for f in .changeset/*.md; do
  # Skip the conventional README marker.
  case "$(basename "$f")" in
    README.md) continue ;;
  esac
  TARGETS+=("$f")
done
shopt -u nullglob

if [ ${#TARGETS[@]} -eq 0 ]; then
  exit 0
fi

# U+2014 EM DASH. The project's docs rule (.claude/rules/documentation.md)
# bans these and prescribes per-context replacements (colon, period,
# semicolon, parens). The auto-generated CHANGELOG is the most-important
# place to enforce this since it ships unedited.
EM_DASH=$'\xe2\x80\x94'

violations=0
for f in "${TARGETS[@]}"; do
  if grep -nF "$EM_DASH" "$f" >/dev/null 2>&1; then
    echo "::error file=$f::em-dash (U+2014) found; replace with colon, period, semicolon, or parens (see .claude/rules/documentation.md)" >&2
    grep -nF "$EM_DASH" "$f" | sed 's/^/  /' >&2
    violations=$((violations + 1))
  fi
done

if [ "$violations" -gt 0 ]; then
  echo "" >&2
  echo "$violations changeset file(s) contain em-dashes. Fix and re-stage." >&2
  exit 1
fi
