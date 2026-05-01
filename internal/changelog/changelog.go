// Package changelog renders a release entry into a per-package
// CHANGELOG.md file in Keep-a-Changelog format.
//
// Insertion is non-destructive: the new entry is prepended above the
// existing version history (the first "## " heading), so any prior
// content — release-please output, hand-written notes, even broken
// markdown — is preserved verbatim. This is the "hard cut" the user
// chose during planning: monorel writes Keep-a-Changelog from now
// forward and leaves whatever was there alone.
//
// If the target file doesn't exist, it's created with a small standard
// preamble. If the file exists but has no "## " heading yet (e.g. a
// pristine preamble-only file), the new entry is appended at the end.
package changelog

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Entry is one CHANGELOG.md release section, ready to render. The
// fields are bucketed by bump level because Keep-a-Changelog wants
// section subheadings, and the bump level is the only categorization
// signal monorel has (changesets carry no commit-type metadata).
type Entry struct {
	// Version is the semver string with the leading "v"
	// (e.g. "v1.6.2"). Render() strips the "v" for the heading
	// since Keep-a-Changelog convention uses bare numbers.
	Version string

	// Date is the release date in YYYY-MM-DD form. Use [Today]
	// to fill from the system clock.
	Date string

	// Major holds the bodies of changesets that requested a
	// major bump for the package this entry is for. One body per
	// element; rendered as bullets under "### Major Changes".
	Major []string

	// Minor holds bodies for minor bumps.
	Minor []string

	// Patch holds bodies for patch bumps.
	Patch []string
}

// Today returns today's UTC date in YYYY-MM-DD form. Use this when
// constructing an Entry from production code; tests should pass a
// fixed date directly.
func Today() string { return time.Now().UTC().Format("2006-01-02") }

// IsEmpty reports whether the entry has no changeset bullets at all.
// An empty Entry shouldn't be rendered into a CHANGELOG.
func (e *Entry) IsEmpty() bool {
	return len(e.Major)+len(e.Minor)+len(e.Patch) == 0
}

// Render formats e as a Keep-a-Changelog version section. The output
// starts with a blank line so it sits cleanly below an existing
// section header.
//
// Format:
//
//	## [X.Y.Z] - YYYY-MM-DD
//
//	### Major Changes
//	- body 1
//	- body 2
//
//	### Minor Changes
//	- ...
//
// Sections with no bullets are omitted. Bodies that contain newlines
// are kept multi-line via two-space indent on continuation lines so
// the markdown renders as one logical bullet.
func (e *Entry) Render() string {
	if e.Version == "" {
		return ""
	}
	version := strings.TrimPrefix(e.Version, "v")
	var b strings.Builder
	fmt.Fprintf(&b, "## [%s] - %s\n", version, e.Date)

	writeSection(&b, "Major Changes", e.Major)
	writeSection(&b, "Minor Changes", e.Minor)
	writeSection(&b, "Patch Changes", e.Patch)

	return b.String()
}

func writeSection(b *strings.Builder, heading string, items []string) {
	if len(items) == 0 {
		return
	}
	fmt.Fprintf(b, "\n### %s\n\n", heading)
	for _, item := range items {
		writeBullet(b, item)
	}
}

// writeBullet emits a markdown bullet whose body may span multiple
// lines. Subsequent lines get a two-space indent so they read as
// continuation of the same bullet under CommonMark.
func writeBullet(b *strings.Builder, body string) {
	body = strings.TrimSpace(body)
	if body == "" {
		b.WriteString("- (no description)\n")
		return
	}
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		if i == 0 {
			fmt.Fprintf(b, "- %s\n", line)
			continue
		}
		if line == "" {
			b.WriteString("\n")
			continue
		}
		fmt.Fprintf(b, "  %s\n", line)
	}
}

// Insert places entry's rendered text into existing, returning the
// updated content. Insertion point: above the first "## " heading.
// If no such heading exists, entry is appended at the end of
// existing.
//
// existing is treated as opaque: it is preserved verbatim except for
// the inserted block. This is the "hard cut" guarantee.
func Insert(existing string, entry *Entry) string {
	if entry == nil || entry.IsEmpty() {
		return existing
	}
	rendered := entry.Render()
	if existing == "" {
		return defaultPreamble() + "\n" + rendered + "\n"
	}

	idx := findFirstH2(existing)
	if idx < 0 {
		// No existing version section. Append, keeping a blank
		// line between the preamble and the new entry.
		return ensureTrailingDoubleNL(existing) + rendered + "\n"
	}

	// Insert just before existing[idx]. Make sure there's a blank
	// line on both sides for readable diffs.
	prefix := existing[:idx]
	suffix := existing[idx:]
	prefix = ensureTrailingDoubleNL(prefix)
	return prefix + rendered + "\n" + suffix
}

// findFirstH2 returns the byte index of the first line that begins
// with "## " (level-2 markdown heading), or -1 if none exists.
func findFirstH2(s string) int {
	for i := 0; i < len(s); {
		end := i
		for end < len(s) && s[end] != '\n' {
			end++
		}
		line := s[i:end]
		if strings.HasPrefix(line, "## ") {
			return i
		}
		// Advance past the newline if any.
		if end < len(s) {
			end++
		}
		i = end
	}
	return -1
}

// ensureTrailingDoubleNL guarantees s ends with exactly one blank line
// (i.e. "...\n\n"). Empty s returns "" unchanged.
func ensureTrailingDoubleNL(s string) string {
	if s == "" {
		return s
	}
	s = strings.TrimRight(s, "\n") + "\n\n"
	return s
}

func defaultPreamble() string {
	return "# Changelog\n\n" +
		"All notable changes to this project are documented in this file.\n\n" +
		"The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),\n" +
		"and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).\n"
}

// WriteFile inserts entry into the file at path. Creates the file
// (and parent directories) if it doesn't exist.
//
// Returns an error if entry is empty (caller should skip the call
// rather than write a no-op file).
func WriteFile(path string, entry *Entry) error {
	if entry == nil {
		return errors.New("nil entry")
	}
	if entry.IsEmpty() {
		return errors.New("empty entry")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
	}
	var existing string
	if data, err := os.ReadFile(path); err == nil {
		existing = string(data)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read %s: %w", path, err)
	}
	updated := Insert(existing, entry)
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
