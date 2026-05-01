package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "monorel.toml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func TestLoad_Valid(t *testing.T) {
	path := writeTempConfig(t, `
[github]
owner = "acme"
repo = "widget"

[packages."github.com/acme/widget"]
tag_prefix = ""
path = "."
changelog = "CHANGELOG.md"

[packages."transports/foo"]
tag_prefix = "transports/foo"
path = "transports/foo"
changelog = "transports/foo/CHANGELOG.md"
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.GitHub.Owner != "acme" || cfg.GitHub.Repo != "widget" {
		t.Errorf("github: %+v", cfg.GitHub)
	}
	if len(cfg.Packages) != 2 {
		t.Errorf("packages: %d, want 2", len(cfg.Packages))
	}
	root := cfg.Packages["github.com/acme/widget"]
	if root.TagPrefix != "" || root.Path != "." || root.Changelog != "CHANGELOG.md" {
		t.Errorf("root: %+v", root)
	}
	if got := root.TagFor("v1.0.0"); got != "v1.0.0" {
		t.Errorf("root.TagFor(v1.0.0) = %q, want v1.0.0", got)
	}

	sub := cfg.Packages["transports/foo"]
	if got := sub.TagFor("v1.0.0"); got != "transports/foo/v1.0.0" {
		t.Errorf("sub.TagFor(v1.0.0) = %q, want transports/foo/v1.0.0", got)
	}
}

func TestLoad_PackageNamesSorted(t *testing.T) {
	path := writeTempConfig(t, `
[github]
owner = "acme"
repo = "w"

[packages.zebra]
tag_prefix = "zebra"
path = "zebra"
changelog = "zebra/CHANGELOG.md"

[packages.apple]
tag_prefix = "apple"
path = "apple"
changelog = "apple/CHANGELOG.md"

[packages.mango]
tag_prefix = "mango"
path = "mango"
changelog = "mango/CHANGELOG.md"
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	got := cfg.PackageNames()
	want := []string{"apple", "mango", "zebra"}
	if len(got) != len(want) {
		t.Fatalf("len: got %d, want %d", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestLoad_RejectsMissingFields(t *testing.T) {
	cases := []struct {
		name    string
		content string
		wantSub string
	}{
		{
			name: "no github owner",
			content: `
[github]
repo = "x"
[packages.foo]
tag_prefix = "foo"
path = "foo"
changelog = "foo/CHANGELOG.md"
`,
			wantSub: "github.owner is required",
		},
		{
			name: "no github repo",
			content: `
[github]
owner = "acme"
[packages.foo]
tag_prefix = "foo"
path = "foo"
changelog = "foo/CHANGELOG.md"
`,
			wantSub: "github.repo is required",
		},
		{
			name: "no packages",
			content: `
[github]
owner = "acme"
repo = "x"
`,
			wantSub: "no packages declared",
		},
		{
			name: "package missing path",
			content: `
[github]
owner = "acme"
repo = "x"
[packages.foo]
tag_prefix = "foo"
changelog = "foo/CHANGELOG.md"
`,
			wantSub: "path is required",
		},
		{
			name: "package missing changelog",
			content: `
[github]
owner = "acme"
repo = "x"
[packages.foo]
tag_prefix = "foo"
path = "foo"
`,
			wantSub: "changelog is required",
		},
		{
			name: "duplicate tag_prefix",
			content: `
[github]
owner = "acme"
repo = "x"
[packages.foo]
tag_prefix = "shared"
path = "foo"
changelog = "foo/CHANGELOG.md"
[packages.bar]
tag_prefix = "shared"
path = "bar"
changelog = "bar/CHANGELOG.md"
`,
			wantSub: "share tag_prefix",
		},
		{
			name: "unknown top-level key",
			content: `
[github]
owner = "acme"
repo = "x"

undeclared_key = 42

[packages.foo]
tag_prefix = "foo"
path = "foo"
changelog = "foo/CHANGELOG.md"
`,
			wantSub: "unknown keys",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := writeTempConfig(t, c.content)
			_, err := Load(path)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", c.wantSub)
			}
			if !strings.Contains(err.Error(), c.wantSub) {
				t.Errorf("err = %v\n want substring %q", err, c.wantSub)
			}
		})
	}
}

func TestLoad_NoPackagesIsErrorIs(t *testing.T) {
	path := writeTempConfig(t, `
[github]
owner = "acme"
repo = "x"
`)
	_, err := Load(path)
	if !errors.Is(err, ErrNoPackages) {
		t.Errorf("errors.Is(err, ErrNoPackages) = false, err = %v", err)
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "missing.toml"))
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !strings.Contains(err.Error(), "read") {
		t.Errorf("err should mention 'read': %v", err)
	}
}
