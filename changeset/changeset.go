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
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/adrg/frontmatter"

	"monorel.disaresta.com/semver"
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
	"pre.json":    true, // pre-release mode state
	"README.md":   true, // optional human-facing README
	".gitkeep":    true, // empty-directory marker
	"config.json": true, // reserved for future config
}

// IsReserved reports whether name is a reserved (non-changeset)
// filename inside the changeset directory. Walkers should skip
// reserved names rather than try to parse them as changesets.
func IsReserved(name string) bool {
	return reservedNames[name]
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
	// adrg/frontmatter doesn't strip a UTF-8 BOM; do it ourselves so
	// authors editing in BOM-prone editors aren't punished.
	raw = bytes.TrimPrefix(raw, []byte("\ufeff"))

	// adrg/frontmatter silently treats "no opening fence" and "no
	// closing fence" the same as "empty frontmatter" \u2014 they all
	// surface as zero bumps. Pre-check the fences so the user gets
	// a precise error pointing at the actual problem.
	if err := checkFrontmatterShape(raw); err != nil {
		return nil, err
	}

	rawBumps := make(map[string]string)
	body, err := frontmatter.Parse(bytes.NewReader(raw), &rawBumps)
	if err != nil {
		return nil, fmt.Errorf("frontmatter: %w", err)
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
		Body:  strings.TrimSpace(string(body)),
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

// checkFrontmatterShape detects the two structural failure modes
// adrg/frontmatter is silent about: an opening "---" fence missing
// entirely, or an opening fence with no matching closing fence on
// its own line. Caller-friendly errors here let users fix typos
// without bouncing through the generic "no packages declared."
func checkFrontmatterShape(raw []byte) error {
	content := strings.TrimLeft(string(raw), " \t\r\n")
	if !strings.HasPrefix(content, "---") {
		return errors.New("missing frontmatter (file must start with '---')")
	}
	rest := content[3:]
	rest = strings.TrimPrefix(rest, "\r")
	rest = strings.TrimPrefix(rest, "\n")
	for _, line := range strings.Split(rest, "\n") {
		if strings.TrimRight(line, "\r") == "---" {
			return nil
		}
	}
	return errors.New("missing closing '---' for frontmatter")
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
