package changeset_test

import (
	"fmt"
	"strings"

	"monorel.disaresta.com/changeset"
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
