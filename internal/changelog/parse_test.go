package changelog_test

import (
	"strings"
	"testing"

	"monorel.disaresta.com/internal/changelog"
)

func TestParseTopEntry_StandardEntry(t *testing.T) {
	content := `# Changelog

## [1.6.0] - 2026-04-30

### Minor Changes

- Adds Lazy() helper.

## [1.5.0] - 2026-03-01

### Patch Changes

- Earlier fix.
`
	v, body, ok := changelog.ParseTopEntry(content)
	if !ok {
		t.Fatal("expected ok=true on standard content")
	}
	if v != "1.6.0" {
		t.Errorf("version = %q, want 1.6.0", v)
	}
	if !strings.Contains(body, "Adds Lazy()") {
		t.Errorf("body missing the entry's bullet:\n%s", body)
	}
	if strings.Contains(body, "Earlier fix") {
		t.Errorf("body leaked older entry:\n%s", body)
	}
}

func TestParseTopEntry_NoH2(t *testing.T) {
	content := "# Changelog\n\nIntroductory text.\n"
	_, _, ok := changelog.ParseTopEntry(content)
	if ok {
		t.Error("expected ok=false when no H2 present")
	}
}

func TestParseTopEntry_OnlyOneEntry(t *testing.T) {
	content := `# Changelog

## [0.1.0] - 2026-04-30

### Minor Changes

- First feature.
`
	v, body, ok := changelog.ParseTopEntry(content)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if v != "0.1.0" {
		t.Errorf("version = %q, want 0.1.0", v)
	}
	if !strings.Contains(body, "First feature.") {
		t.Errorf("body missing entry content:\n%s", body)
	}
}

func TestParseTopEntry_PreReleaseVersion(t *testing.T) {
	content := "## [1.6.0-rc.1] - 2026-04-30\n\n### Minor Changes\n\n- rc body.\n"
	v, _, ok := changelog.ParseTopEntry(content)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if v != "1.6.0-rc.1" {
		t.Errorf("version = %q, want 1.6.0-rc.1", v)
	}
}

func TestParseTopEntry_MalformedHeading(t *testing.T) {
	content := "## something else\n\nbody\n"
	v, _, ok := changelog.ParseTopEntry(content)
	if !ok {
		t.Fatal("expected ok=true (any H2 counts)")
	}
	if v != "" {
		t.Errorf("version from malformed heading = %q, want empty", v)
	}
}
