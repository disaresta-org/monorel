package changelog_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"monorel.disaresta.com/changelog"
)

func TestEntry_Render_AllSections(t *testing.T) {
	e := &changelog.Entry{
		Version: "v1.6.2",
		Date:    "2026-04-30",
		Major:   []string{"Drop legacy API."},
		Minor:   []string{"Add foo field.", "Add bar field."},
		Patch:   []string{"Fix off-by-one."},
	}
	out := e.Render()

	for _, want := range []string{
		"## [1.6.2] - 2026-04-30",
		"### Major Changes",
		"- Drop legacy API.",
		"### Minor Changes",
		"- Add foo field.",
		"- Add bar field.",
		"### Patch Changes",
		"- Fix off-by-one.",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("Render() missing %q\nfull:\n%s", want, out)
		}
	}
	// "v" should be stripped from the heading.
	if strings.Contains(out, "## [v1.6.2]") {
		t.Errorf("Render() should strip 'v' from version heading:\n%s", out)
	}
}

func TestEntry_Render_OmitsEmptySections(t *testing.T) {
	e := &changelog.Entry{
		Version: "v0.1.0",
		Date:    "2026-04-30",
		Patch:   []string{"Bug fix."},
	}
	out := e.Render()
	if strings.Contains(out, "Major Changes") || strings.Contains(out, "Minor Changes") {
		t.Errorf("expected only Patch section; got:\n%s", out)
	}
	if !strings.Contains(out, "### Patch Changes") {
		t.Errorf("expected Patch Changes section:\n%s", out)
	}
}

func TestEntry_Render_MultiLineBody(t *testing.T) {
	e := &changelog.Entry{
		Version: "v1.0.0",
		Date:    "2026-04-30",
		Minor: []string{
			"First line.\nSecond line.\n\nThird paragraph.",
		},
	}
	out := e.Render()
	want := "- First line.\n  Second line.\n\n  Third paragraph.\n"
	if !strings.Contains(out, want) {
		t.Errorf("expected indented continuation:\nwant: %q\n\ngot:\n%s", want, out)
	}
}

func TestEntry_IsEmpty(t *testing.T) {
	if !(&changelog.Entry{}).IsEmpty() {
		t.Error("default Entry should be empty")
	}
	if (&changelog.Entry{Patch: []string{"x"}}).IsEmpty() {
		t.Error("Entry with Patch should not be empty")
	}
}

func TestInsert_NewFile(t *testing.T) {
	e := &changelog.Entry{
		Version: "v0.1.0",
		Date:    "2026-04-30",
		Minor:   []string{"First feature."},
	}
	out := changelog.Insert("", e)
	if !strings.HasPrefix(out, "# Changelog") {
		t.Errorf("new file should start with preamble:\n%s", out)
	}
	if !strings.Contains(out, "## [0.1.0] - 2026-04-30") {
		t.Errorf("new file missing version heading:\n%s", out)
	}
	if !strings.Contains(out, "### Minor Changes") {
		t.Errorf("new file missing section heading:\n%s", out)
	}
}

func TestInsert_PreservesExistingHistory(t *testing.T) {
	existing := `# Changelog

All notable changes to this project are documented in this file.

## [1.5.0] - 2026-04-01

### Minor Changes

- Earlier feature.

## [1.4.0] - 2026-03-01

### Patch Changes

- Earlier fix.
`
	e := &changelog.Entry{
		Version: "v1.6.0",
		Date:    "2026-04-30",
		Minor:   []string{"New feature."},
	}
	out := changelog.Insert(existing, e)

	// New entry must appear ABOVE the existing 1.5.0 entry.
	idxNew := strings.Index(out, "## [1.6.0]")
	idxOld := strings.Index(out, "## [1.5.0]")
	if idxNew < 0 || idxOld < 0 {
		t.Fatalf("missing headings: idxNew=%d idxOld=%d\n%s", idxNew, idxOld, out)
	}
	if idxNew >= idxOld {
		t.Errorf("new entry should be ABOVE old: idxNew=%d, idxOld=%d", idxNew, idxOld)
	}
	// All existing content must still be present verbatim.
	if !strings.Contains(out, "Earlier feature.") || !strings.Contains(out, "Earlier fix.") {
		t.Errorf("existing content not preserved:\n%s", out)
	}
}

func TestInsert_PreambleOnly(t *testing.T) {
	existing := "# Changelog\n\nIntroductory text.\n"
	e := &changelog.Entry{
		Version: "v0.1.0",
		Date:    "2026-04-30",
		Patch:   []string{"First fix."},
	}
	out := changelog.Insert(existing, e)
	if !strings.HasPrefix(out, "# Changelog") {
		t.Errorf("preamble lost:\n%s", out)
	}
	if !strings.Contains(out, "Introductory text.") {
		t.Errorf("intro text lost:\n%s", out)
	}
	if !strings.Contains(out, "## [0.1.0]") {
		t.Errorf("entry missing:\n%s", out)
	}
}

func TestInsert_ReleasePleaseFormatPreserved(t *testing.T) {
	// Snapshot of release-please-style content. monorel must not
	// rewrite it; new entries go above, old entries stay verbatim
	// (including the (compare-link) shape that Keep-a-Changelog
	// doesn't use).
	existing := `# Changelog

## [1.6.1](https://github.com/foo/bar/compare/v1.6.0...v1.6.1) (2025-12-30)


### Bug Fixes

* something ([abc1234](https://github.com/foo/bar/commit/abc1234))

## [1.6.0](https://github.com/foo/bar/compare/v1.5.0...v1.6.0) (2025-12-29)


### Features

* a thing
`
	e := &changelog.Entry{
		Version: "v1.7.0",
		Date:    "2026-04-30",
		Minor:   []string{"New monorel-format feature."},
	}
	out := changelog.Insert(existing, e)

	// The release-please content must survive byte-for-byte (with
	// the only change being the prepended new entry above it).
	if !strings.Contains(out, "## [1.6.1](https://github.com/foo/bar/compare/v1.6.0...v1.6.1) (2025-12-30)") {
		t.Errorf("release-please header lost:\n%s", out)
	}
	if !strings.Contains(out, "* something ([abc1234](https://github.com/foo/bar/commit/abc1234))") {
		t.Errorf("release-please bullet lost:\n%s", out)
	}
	// New entry comes first.
	idxNew := strings.Index(out, "## [1.7.0]")
	idxOld := strings.Index(out, "## [1.6.1]")
	if idxNew < 0 || idxOld < 0 || idxNew >= idxOld {
		t.Errorf("ordering wrong: idxNew=%d idxOld=%d", idxNew, idxOld)
	}
}

func TestInsert_NilOrEmptyEntryNoOp(t *testing.T) {
	existing := "# Changelog\n## [1.0.0] - 2026-01-01\n"
	if got := changelog.Insert(existing, nil); got != existing {
		t.Errorf("nil entry should be no-op")
	}
	if got := changelog.Insert(existing, &changelog.Entry{Version: "v1.0.0", Date: "2026-04-30"}); got != existing {
		t.Errorf("empty entry should be no-op")
	}
}

func TestWriteFile_CreatesAndAppends(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "transports", "foo", "CHANGELOG.md")

	e := &changelog.Entry{
		Version: "v0.1.0",
		Date:    "2026-04-30",
		Minor:   []string{"First feature."},
	}
	if err := changelog.WriteFile(path, e); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if !strings.Contains(string(data), "## [0.1.0]") {
		t.Errorf("written content missing version:\n%s", data)
	}

	// Second write: the second entry must appear above the first.
	e2 := &changelog.Entry{
		Version: "v0.2.0",
		Date:    "2026-05-01",
		Minor:   []string{"Second feature."},
	}
	if err := changelog.WriteFile(path, e2); err != nil {
		t.Fatalf("WriteFile second: %v", err)
	}
	data2, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	idx2 := strings.Index(string(data2), "## [0.2.0]")
	idx1 := strings.Index(string(data2), "## [0.1.0]")
	if idx2 < 0 || idx1 < 0 || idx2 >= idx1 {
		t.Errorf("ordering wrong after second write: idx2=%d idx1=%d\n%s", idx2, idx1, data2)
	}
}

func TestWriteFile_RejectsEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "CHANGELOG.md")
	if err := changelog.WriteFile(path, nil); err == nil {
		t.Error("expected error for nil entry")
	}
	if err := changelog.WriteFile(path, &changelog.Entry{Version: "v1"}); err == nil {
		t.Error("expected error for empty entry")
	}
}

func TestToday_Format(t *testing.T) {
	got := changelog.Today()
	if len(got) != len("YYYY-MM-DD") {
		t.Errorf("Today() = %q, want YYYY-MM-DD", got)
	}
	if got[4] != '-' || got[7] != '-' {
		t.Errorf("Today() = %q, expected dashes at positions 4 and 7", got)
	}
}
