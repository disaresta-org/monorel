package changelog_test

import (
	"fmt"

	"monorel.disaresta.com/changelog"
)

// ExampleInsert prepends a release entry above the existing version
// history of a CHANGELOG.md. The entry's body is monorel-shaped
// (Keep-a-Changelog headings) but the existing content is preserved
// verbatim regardless of whatever format it was previously written in.
func ExampleInsert() {
	existing := `# Changelog

## [1.6.1] - 2026-04-30

Bumped some deps.
`

	entry := &changelog.Entry{
		Version: "v1.7.0",
		Date:    "2026-05-01",
		Minor:   []string{"Adds Lazy() helper for deferred field evaluation."},
		Patch:   []string{"Fixes a panic when a transport's Send returns nil."},
	}

	updated := changelog.Insert(existing, entry)
	fmt.Print(updated)
	// Output:
	// # Changelog
	//
	// ## [1.7.0] - 2026-05-01
	//
	// ### Minor Changes
	//
	// - Adds Lazy() helper for deferred field evaluation.
	//
	// ### Patch Changes
	//
	// - Fixes a panic when a transport's Send returns nil.
	//
	// ## [1.6.1] - 2026-04-30
	//
	// Bumped some deps.
}
