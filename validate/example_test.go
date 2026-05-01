package validate_test

import (
	"fmt"
	"os"
	"path/filepath"

	"monorel.disaresta.com/validate"
)

// ExampleRun walks a monorel.toml + the changeset directory and
// reports every issue in one pass. Unlike the rest of monorel's
// loaders (which fail-fast on the first error), Run is fault-tolerant
// by design: it keeps going so authors fix every issue in one
// round-trip.
//
// The example sets up a deliberately-broken layout (a typo in a
// changeset's package key) so the output has a finding to show.
func ExampleRun() {
	dir, _ := os.MkdirTemp("", "monorel-validate")
	defer os.RemoveAll(dir)

	_ = os.WriteFile(filepath.Join(dir, "monorel.toml"), []byte(`
[forge]
owner = "acme"
repo = "widget"

[packages."widget"]
tag_prefix = ""
path = "."
changelog = "CHANGELOG.md"
`), 0o644)

	csDir := filepath.Join(dir, ".changeset")
	_ = os.MkdirAll(csDir, 0o755)
	_ = os.WriteFile(filepath.Join(csDir, "typo.md"), []byte(`---
"widgett": patch
---

Typo: widget vs widgett.
`), 0o644)

	findings := validate.Run(validate.Inputs{
		ConfigPath: filepath.Join(dir, "monorel.toml"),
	})
	for _, f := range findings {
		fmt.Printf("%s [%s] %s\n", f.Severity, f.Code, f.Package)
	}
	fmt.Println("has errors:", validate.HasErrors(findings))
	// Output:
	// error [changeset_unknown_package] widgett
	// has errors: true
}

// ExampleRun_clean shows the success case: a well-formed monorel.toml
// with no pending changesets returns no findings.
func ExampleRun_clean() {
	dir, _ := os.MkdirTemp("", "monorel-validate-clean")
	defer os.RemoveAll(dir)

	_ = os.WriteFile(filepath.Join(dir, "monorel.toml"), []byte(`
[forge]
owner = "acme"
repo = "widget"

[packages."widget"]
tag_prefix = ""
path = "."
changelog = "CHANGELOG.md"
`), 0o644)

	findings := validate.Run(validate.Inputs{
		ConfigPath: filepath.Join(dir, "monorel.toml"),
	})
	fmt.Println("findings:", len(findings))
	fmt.Println("has errors:", validate.HasErrors(findings))
	// Output:
	// findings: 0
	// has errors: false
}
