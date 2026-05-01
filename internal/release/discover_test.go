package release_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"monorel.disaresta.com/config"
	"monorel.disaresta.com/internal/git"
	"monorel.disaresta.com/internal/release"
)

func writeFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	path := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDiscoverPublishables_Empty(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{
		Provider: config.ProviderConfig{Owner: "x", Repo: "y"},
		Packages: map[string]config.PackageConfig{
			"foo": {TagPrefix: "transports/foo", Path: "transports/foo", Changelog: "transports/foo/CHANGELOG.md"},
		},
	}
	repo := git.NewFake() // no tags
	got, err := release.DiscoverPublishables(cfg, repo, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("got %d, want 0", len(got))
	}
}

func TestDiscoverPublishables_StableMode(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{
		Provider: config.ProviderConfig{Owner: "x", Repo: "y"},
		Packages: map[string]config.PackageConfig{
			"foo": {TagPrefix: "transports/foo", Path: "transports/foo", Changelog: "transports/foo/CHANGELOG.md"},
		},
	}
	writeFile(t, dir, "transports/foo/CHANGELOG.md", `# Changelog

## [1.6.0] - 2026-04-30

### Minor Changes

- Adds Lazy() helper.

## [1.5.0] - 2026-03-01

### Patch Changes

- Earlier fix.
`)

	repo := git.NewFake()
	if err := repo.CreateTag("transports/foo/v1.6.0", ""); err != nil {
		t.Fatal(err)
	}

	got, err := release.DiscoverPublishables(cfg, repo, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d, want 1", len(got))
	}
	if got[0].Tag != "transports/foo/v1.6.0" {
		t.Errorf("Tag = %q", got[0].Tag)
	}
	if got[0].Prerelease {
		t.Error("Prerelease = true, want false (no '-' in version)")
	}
	if got[0].Body == "" {
		t.Error("Body empty, want changelog content")
	}
	// The body should contain the Minor Changes section but NOT the
	// older [1.5.0] entry.
	if !strings.Contains(got[0].Body, "Adds Lazy()") {
		t.Errorf("body missing top-entry content: %q", got[0].Body)
	}
	if strings.Contains(got[0].Body, "Earlier fix") {
		t.Errorf("body leaked older-entry content: %q", got[0].Body)
	}
}

func TestDiscoverPublishables_PreReleaseFlag(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{
		Provider: config.ProviderConfig{Owner: "x", Repo: "y"},
		Packages: map[string]config.PackageConfig{
			"foo": {TagPrefix: "transports/foo", Path: "transports/foo", Changelog: "transports/foo/CHANGELOG.md"},
		},
	}
	repo := git.NewFake()
	if err := repo.CreateTag("transports/foo/v1.6.0-rc.0", ""); err != nil {
		t.Fatal(err)
	}

	got, err := release.DiscoverPublishables(cfg, repo, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d, want 1", len(got))
	}
	if !got[0].Prerelease {
		t.Error("pre-release tag (suffix -rc.0) should be flagged")
	}
}

func TestDiscoverPublishables_BareTagRoot(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{
		Provider: config.ProviderConfig{Owner: "x", Repo: "y"},
		Packages: map[string]config.PackageConfig{
			"core": {TagPrefix: "", Path: ".", Changelog: "CHANGELOG.md"},
			"foo":  {TagPrefix: "transports/foo", Path: "transports/foo", Changelog: "transports/foo/CHANGELOG.md"},
		},
	}
	writeFile(t, dir, "CHANGELOG.md", "# Changelog\n\n## [1.0.0] - 2026-04-30\n\n### Major Changes\n\n- Initial.\n")
	writeFile(t, dir, "transports/foo/CHANGELOG.md", "# Changelog\n\n## [0.1.0] - 2026-04-30\n\n### Minor Changes\n\n- Initial.\n")

	repo := git.NewFake()
	if err := repo.CreateTag("v1.0.0", ""); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTag("transports/foo/v0.1.0", ""); err != nil {
		t.Fatal(err)
	}

	got, err := release.DiscoverPublishables(cfg, repo, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d, want 2", len(got))
	}
	// Tags are sorted lexicographically; transports/... comes before v...
	gotTags := []string{got[0].Tag, got[1].Tag}
	want := map[string]bool{"v1.0.0": true, "transports/foo/v0.1.0": true}
	for _, tag := range gotTags {
		if !want[tag] {
			t.Errorf("unexpected tag %q in result", tag)
		}
	}
}
