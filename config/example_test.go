package config_test

import (
	"fmt"
	"os"
	"path/filepath"

	"monorel.disaresta.com/config"
)

// ExampleLoad parses a monorel.toml from disk. The schema is validated
// at load time; unknown fields, missing required keys, and duplicate
// tag prefixes all return an error here rather than later in the
// release pipeline.
func ExampleLoad() {
	dir, _ := os.MkdirTemp("", "monorel-config")
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "monorel.toml")
	_ = os.WriteFile(path, []byte(`
[provider]
name = "github"
owner = "acme"
repo = "widget"

[packages."github.com/acme/widget"]
tag_prefix = ""
path = "."
changelog = "CHANGELOG.md"

[packages."transports/zerolog"]
tag_prefix = "transports/zerolog"
path = "transports/zerolog"
changelog = "transports/zerolog/CHANGELOG.md"
`), 0o644)

	cfg, err := config.Load(path)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println("provider:", cfg.Provider.Name, cfg.Provider.Owner+"/"+cfg.Provider.Repo)
	for _, name := range cfg.PackageNames() {
		pkg := cfg.Packages[name]
		fmt.Printf("- %s → tag %sv1.2.3\n", name, pkg.FullTagPrefix())
	}
	// Output:
	// provider: github acme/widget
	// - github.com/acme/widget → tag v1.2.3
	// - transports/zerolog → tag transports/zerolog/v1.2.3
}
