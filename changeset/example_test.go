package changeset_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"monorel.disaresta.com/changeset"
	"monorel.disaresta.com/semver"
)

// ExampleParse reads one .changeset/<name>.md file's contents and
// returns the structured frontmatter + body. The name argument is
// the filename minus the .md suffix; it propagates into Changeset.Name
// so callers can correlate parse results with their on-disk source.
func ExampleParse() {
	src := `---
"transports/zerolog": minor
"go.loglayer.dev": patch
---

Adds Lazy() helper for deferred field evaluation. Pass-through
fix in the root.
`
	cs, err := changeset.Parse(strings.NewReader(src), "quick-otter")
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println("name:", cs.Name)
	for _, pkg := range cs.PackageNames() {
		fmt.Printf("- %s: %s\n", pkg, cs.Bumps[pkg])
	}
	fmt.Println("body:", strings.SplitN(cs.Body, "\n", 2)[0])
	// Output:
	// name: quick-otter
	// - go.loglayer.dev: patch
	// - transports/zerolog: minor
	// body: Adds Lazy() helper for deferred field evaluation. Pass-through
}

// ExampleChangeset_WriteFile demonstrates the round trip: construct
// a Changeset, write it under .changeset/<name>.md, and parse it
// back. This is the pattern an authoring tool (IDE plugin, custom
// `monorel add` wrapper, etc.) would follow to produce changesets
// programmatically.
func ExampleChangeset_WriteFile() {
	dir, _ := os.MkdirTemp("", "monorel-changesets")
	defer os.RemoveAll(dir)

	cs := &changeset.Changeset{
		Name: "quick-otter",
		Bumps: map[string]semver.BumpLevel{
			"transports/zerolog": semver.Minor,
		},
		Body: "Adds Lazy() helper.",
	}
	if err := cs.WriteFile(dir); err != nil {
		fmt.Println("write error:", err)
		return
	}

	// Round-trip: read the file back via LoadAll.
	loaded, err := changeset.LoadAll(dir)
	if err != nil {
		fmt.Println("load error:", err)
		return
	}
	for _, c := range loaded {
		fmt.Printf("%s: %d package(s), body=%q\n", c.Name, len(c.Bumps), c.Body)
	}

	// Verify the file landed where expected.
	if _, err := os.Stat(filepath.Join(dir, "quick-otter.md")); err != nil {
		fmt.Println("file missing:", err)
	}
	// Output:
	// quick-otter: 1 package(s), body="Adds Lazy() helper."
}
