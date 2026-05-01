// Package changeset reads and writes the .changeset/<name>.md files
// that drive monorel releases.
//
// File format:
//
//	---
//	"package-a": minor
//	"package-b": patch
//	---
//
//	Body text becomes the changelog entry for these packages.
//
// The frontmatter is YAML; the body is markdown that monorel passes
// through verbatim into per-package CHANGELOG.md sections. Files live
// in .changeset/<name>.md at the repo root; .changeset/pre.json is
// pre-release-mode state and is not a changeset.
package changeset

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/disaresta-org/monorel/internal/semver"
)

// Changeset is one parsed .changeset/<name>.md file.
type Changeset struct {
	// Name is the filename without the ".md" extension and without
	// the ".changeset/" directory prefix. Used to delete the file
	// after a release consumes it, and as a stable identifier in
	// logs and PR bodies.
	Name string

	// Bumps maps package names (matching keys in monorel.toml's
	// [packages] table) to the requested bump level for this
	// changeset. Always non-empty for a valid changeset.
	Bumps map[string]semver.BumpLevel

	// Body is the markdown text after the frontmatter. Trimmed of
	// leading/trailing whitespace. May be empty (parser tolerates
	// it; release writes a placeholder line in that case).
	Body string
}

// PackageNames returns the package names this changeset affects in
// lexicographic order.
func (c *Changeset) PackageNames() []string {
	names := make([]string, 0, len(c.Bumps))
	for name := range c.Bumps {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Reserved filenames inside .changeset/ that are NOT changesets.
// LoadAll skips these.
var reservedNames = map[string]bool{
	"pre.json":     true, // pre-release mode state
	"README.md":    true, // optional human-facing README
	".gitkeep":     true, // empty-directory marker
	"config.json":  true, // reserved for future config
}

// LoadAll reads every changeset file under dir (excluding reserved
// names) and returns them sorted by name. Missing dir is not an
// error: it just yields zero changesets.
func LoadAll(dir string) ([]*Changeset, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read %s: %w", dir, err)
	}

	var changesets []*Changeset
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if reservedNames[entry.Name()] {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		f, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("open %s: %w", path, err)
		}
		cs, err := Parse(f, strings.TrimSuffix(entry.Name(), ".md"))
		f.Close()
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		changesets = append(changesets, cs)
	}
	sort.Slice(changesets, func(i, j int) bool {
		return changesets[i].Name < changesets[j].Name
	})
	return changesets, nil
}

// Parse reads a single changeset from r. name is the changeset's
// filename minus ".md".
//
// Errors are returned for: missing or malformed frontmatter, YAML
// parse failures, empty bumps map, and unknown bump levels.
func Parse(r io.Reader, name string) (*Changeset, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	frontmatter, body, err := splitFrontmatter(string(raw))
	if err != nil {
		return nil, err
	}

	// YAML decodes "key: value" pairs into a map preserving order
	// only via yaml.Node; for our purposes we just need the unordered
	// map.
	rawBumps := make(map[string]string)
	if err := yaml.Unmarshal([]byte(frontmatter), &rawBumps); err != nil {
		return nil, fmt.Errorf("frontmatter yaml: %w", err)
	}
	if len(rawBumps) == 0 {
		return nil, errors.New("frontmatter declares no packages")
	}

	bumps := make(map[string]semver.BumpLevel, len(rawBumps))
	for pkg, levelStr := range rawBumps {
		level, err := semver.ParseBumpLevel(levelStr)
		if err != nil {
			return nil, fmt.Errorf("package %q: %w", pkg, err)
		}
		bumps[pkg] = level
	}

	return &Changeset{
		Name:  name,
		Bumps: bumps,
		Body:  strings.TrimSpace(body),
	}, nil
}

// Write serializes a changeset back to the .changeset/<name>.md
// format. Bumps are emitted in lexicographic order so file content
// is deterministic across runs.
func (c *Changeset) Write(w io.Writer) error {
	if len(c.Bumps) == 0 {
		return errors.New("changeset has no bumps")
	}
	if _, err := io.WriteString(w, "---\n"); err != nil {
		return err
	}
	for _, pkg := range c.PackageNames() {
		// Always quote package names: TOML/YAML map keys with
		// special chars (slashes, dots) would otherwise need
		// escaping. Quoting universally keeps the format simple.
		level := c.Bumps[pkg]
		line := fmt.Sprintf("%q: %s\n", pkg, level)
		if _, err := io.WriteString(w, line); err != nil {
			return err
		}
	}
	if _, err := io.WriteString(w, "---\n\n"); err != nil {
		return err
	}
	if _, err := io.WriteString(w, strings.TrimSpace(c.Body)); err != nil {
		return err
	}
	if _, err := io.WriteString(w, "\n"); err != nil {
		return err
	}
	return nil
}

// WriteFile is a convenience wrapper that writes the changeset to
// dir/<name>.md, creating dir if it doesn't exist.
func (c *Changeset) WriteFile(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	path := filepath.Join(dir, c.Name+".md")
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer f.Close()
	if err := c.Write(f); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// splitFrontmatter separates "---\n<yaml>\n---\n<body>" into its two
// parts. Returns ("", "", error) if the document doesn't start with
// "---" on its own line.
func splitFrontmatter(content string) (frontmatter, body string, err error) {
	// Tolerate UTF-8 BOM and leading whitespace before the first ---.
	trimmed := strings.TrimLeft(content, " \t\n\r\ufeff")
	if !strings.HasPrefix(trimmed, "---\n") && trimmed != "---" && !strings.HasPrefix(trimmed, "---\r\n") {
		return "", "", errors.New("missing frontmatter (file must start with '---')")
	}
	// Strip the opening fence.
	rest := strings.TrimPrefix(trimmed, "---\n")
	rest = strings.TrimPrefix(rest, "---\r\n")
	// Find the closing fence: a line that is exactly "---".
	closeIdx := -1
	scanner := newLineScanner(rest)
	for scanner.next() {
		line := scanner.line()
		if line == "---" {
			closeIdx = scanner.start()
			break
		}
	}
	if closeIdx == -1 {
		return "", "", errors.New("missing closing '---' for frontmatter")
	}
	frontmatter = rest[:closeIdx]
	bodyStart := closeIdx + len("---")
	if bodyStart < len(rest) && rest[bodyStart] == '\n' {
		bodyStart++
	} else if bodyStart+1 < len(rest) && rest[bodyStart] == '\r' && rest[bodyStart+1] == '\n' {
		bodyStart += 2
	}
	body = rest[bodyStart:]
	return frontmatter, body, nil
}

// lineScanner is a tiny line iterator over a string that records the
// byte offset where each line begins. We can't use bufio.Scanner here
// because we need byte offsets to slice the original string.
type lineScanner struct {
	s     string
	pos   int
	begin int
	end   int
}

func newLineScanner(s string) *lineScanner { return &lineScanner{s: s} }

func (l *lineScanner) next() bool {
	if l.pos >= len(l.s) {
		return false
	}
	l.begin = l.pos
	for l.pos < len(l.s) && l.s[l.pos] != '\n' {
		l.pos++
	}
	l.end = l.pos
	if l.pos < len(l.s) {
		l.pos++ // consume the newline
	}
	return true
}

func (l *lineScanner) line() string {
	end := l.end
	// Trim a trailing \r so "\r\n" line endings normalize.
	if end > l.begin && l.s[end-1] == '\r' {
		end--
	}
	return l.s[l.begin:end]
}

// start returns the byte offset of the current line's first byte
// in the original string.
func (l *lineScanner) start() int { return l.begin }
