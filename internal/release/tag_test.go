package release_test

import (
	"errors"
	"testing"

	"monorel.disaresta.com/config"
	"monorel.disaresta.com/internal/git"
	"monorel.disaresta.com/internal/release"
)

// pkgConfig is a small helper for these tests.
func pkgConfig(prefix, path string) config.PackageConfig {
	return config.PackageConfig{
		TagPrefix: prefix,
		Path:      path,
		Changelog: path + "/CHANGELOG.md",
	}
}

func TestTag_ReadsTrailersAndCreatesTags(t *testing.T) {
	f := git.NewFake()
	// Simulate a release commit produced by Apply: subject + trailers.
	if err := f.Add("dummy"); err != nil {
		t.Fatal(err)
	}
	if err := f.Commit("chore(release): transports/foo v1.7.0\n\nmonorel-Release: transports/foo v1.7.0\nmonorel-PreRelease: false\n"); err != nil {
		t.Fatal(err)
	}
	res, err := release.Tag(release.TagOptions{
		Config: &config.Config{
			Provider: config.ProviderConfig{Owner: "x", Repo: "y"},
			Packages: map[string]config.PackageConfig{
				"transports/foo": pkgConfig("transports/foo", "transports/foo"),
			},
		},
		Repo: f,
	})
	if err != nil {
		t.Fatalf("Tag: %v", err)
	}
	want := "transports/foo/v1.7.0"
	if len(res.Releases) != 1 || res.Releases[0].Tag != want {
		t.Errorf("Releases = %+v, want one tag %q", res.Releases, want)
	}
	if !contains(f.Tags, want) {
		t.Errorf("repo Tags missing %q (have %v)", want, f.Tags)
	}
	if res.Releases[0].Prerelease {
		t.Error("Prerelease = true, want false")
	}
}

func TestTag_NoReleaseCommit(t *testing.T) {
	f := git.NewFake()
	// HEAD has no commits → empty message → no trailers.
	_, err := release.Tag(release.TagOptions{
		Config: &config.Config{Provider: config.ProviderConfig{Owner: "x", Repo: "y"}},
		Repo:   f,
	})
	if !errors.Is(err, release.ErrNoReleaseCommit) {
		t.Fatalf("err = %v, want ErrNoReleaseCommit", err)
	}
}

func TestTag_UnknownPackage(t *testing.T) {
	f := git.NewFake()
	if err := f.Add("dummy"); err != nil {
		t.Fatal(err)
	}
	if err := f.Commit("chore(release): foo v1.0.0\n\nmonorel-Release: foo v1.0.0\nmonorel-PreRelease: false\n"); err != nil {
		t.Fatal(err)
	}
	// Config has no "foo" package.
	_, err := release.Tag(release.TagOptions{
		Config: &config.Config{
			Provider: config.ProviderConfig{Owner: "x", Repo: "y"},
			Packages: map[string]config.PackageConfig{},
		},
		Repo: f,
	})
	if !errors.Is(err, release.ErrUnknownReleasedPackage) {
		t.Fatalf("err = %v, want ErrUnknownReleasedPackage", err)
	}
}

func TestTag_TagAlreadyExists(t *testing.T) {
	f := git.NewFake()
	f.Tags = []string{"v1.0.0"}
	if err := f.Add("dummy"); err != nil {
		t.Fatal(err)
	}
	if err := f.Commit("chore(release): root v1.0.0\n\nmonorel-Release: root v1.0.0\nmonorel-PreRelease: false\n"); err != nil {
		t.Fatal(err)
	}
	_, err := release.Tag(release.TagOptions{
		Config: &config.Config{
			Provider: config.ProviderConfig{Owner: "x", Repo: "y"},
			Packages: map[string]config.PackageConfig{
				"root": pkgConfig("", "."),
			},
		},
		Repo: f,
	})
	if !errors.Is(err, release.ErrTagExists) {
		t.Fatalf("err = %v, want ErrTagExists", err)
	}
}

func TestTag_PreReleaseFlagPropagates(t *testing.T) {
	f := git.NewFake()
	if err := f.Add("dummy"); err != nil {
		t.Fatal(err)
	}
	if err := f.Commit("chore(release): foo v1.0.0-rc.0 (pre-release)\n\nmonorel-Release: foo v1.0.0-rc.0\nmonorel-PreRelease: true\n"); err != nil {
		t.Fatal(err)
	}
	res, err := release.Tag(release.TagOptions{
		Config: &config.Config{
			Provider: config.ProviderConfig{Owner: "x", Repo: "y"},
			Packages: map[string]config.PackageConfig{
				"foo": pkgConfig("transports/foo", "transports/foo"),
			},
		},
		Repo: f,
	})
	if err != nil {
		t.Fatalf("Tag: %v", err)
	}
	if !res.Releases[0].Prerelease {
		t.Error("Prerelease = false, want true")
	}
	if res.Releases[0].Tag != "transports/foo/v1.0.0-rc.0" {
		t.Errorf("Tag = %q, want transports/foo/v1.0.0-rc.0", res.Releases[0].Tag)
	}
}

func TestTag_MultiPackage_PreservesOrder(t *testing.T) {
	f := git.NewFake()
	// Multi-package release commit: two trailers in plan order.
	msg := "chore(release): pkg-a v1.0.0, pkg-b v2.0.0\n" +
		"\n" +
		"monorel-Release: pkg-a v1.0.0\n" +
		"monorel-Release: pkg-b v2.0.0\n" +
		"monorel-PreRelease: false\n"
	if err := f.Add("dummy"); err != nil {
		t.Fatal(err)
	}
	if err := f.Commit(msg); err != nil {
		t.Fatal(err)
	}
	res, err := release.Tag(release.TagOptions{
		Config: &config.Config{
			Provider: config.ProviderConfig{Owner: "x", Repo: "y"},
			Packages: map[string]config.PackageConfig{
				"pkg-a": pkgConfig("a", "a"),
				"pkg-b": pkgConfig("b", "b"),
			},
		},
		Repo: f,
	})
	if err != nil {
		t.Fatalf("Tag: %v", err)
	}
	wantTags := []string{"a/v1.0.0", "b/v2.0.0"}
	got := []string{res.Releases[0].Tag, res.Releases[1].Tag}
	if got[0] != wantTags[0] || got[1] != wantTags[1] {
		t.Errorf("tags = %v, want %v (in order)", got, wantTags)
	}
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}
