package changelog_test

import (
	"fmt"
	"os"
	"path/filepath"

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

// ExampleWriteFile shows the path most callers actually take:
// produce an Entry and write it to disk. WriteFile reads the
// existing file (if any), runs Insert, and writes the result back
// atomically. A non-existent target is created with a default
// Keep-a-Changelog preamble.
func ExampleWriteFile() {
	dir, _ := os.MkdirTemp("", "monorel-changelog")
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "CHANGELOG.md")

	first := &changelog.Entry{
		Version: "v1.0.0",
		Date:    "2026-04-29",
		Major:   []string{"Initial release."},
	}
	if err := changelog.WriteFile(path, first); err != nil {
		fmt.Println("error:", err)
		return
	}

	second := &changelog.Entry{
		Version: "v1.1.0",
		Date:    "2026-05-01",
		Minor:   []string{"Add WithFields helper."},
	}
	if err := changelog.WriteFile(path, second); err != nil {
		fmt.Println("error:", err)
		return
	}

	// The newer entry lands above the older one; the preamble that
	// was written on the first call is preserved.
	got, _ := os.ReadFile(path)
	fmt.Print(string(got))
	// Output:
	// # Changelog
	//
	// All notable changes to this project are documented in this file.
	//
	// The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
	// and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).
	//
	// ## [1.1.0] - 2026-05-01
	//
	// ### Minor Changes
	//
	// - Add WithFields helper.
	//
	// ## [1.0.0] - 2026-04-29
	//
	// ### Major Changes
	//
	// - Initial release.
}
