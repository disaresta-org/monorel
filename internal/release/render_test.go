package release_test

import (
	"strings"
	"testing"

	"github.com/disaresta-org/monorel/internal/changeset"
	"github.com/disaresta-org/monorel/internal/config"
	"github.com/disaresta-org/monorel/internal/plan"
	"github.com/disaresta-org/monorel/internal/release"
	"github.com/disaresta-org/monorel/internal/semver"
)

func TestRenderPreview_Empty(t *testing.T) {
	got := release.RenderPreview(nil, "2026-04-30")
	if !strings.Contains(got, "No pending changesets") {
		t.Errorf("nil plan body: %s", got)
	}
	got = release.RenderPreview(&plan.ReleasePlan{}, "2026-04-30")
	if !strings.Contains(got, "No pending changesets") {
		t.Errorf("empty plan body: %s", got)
	}
}

func TestRenderPreview_Stable(t *testing.T) {
	cs1 := &changeset.Changeset{
		Name:  "first",
		Bumps: map[string]semver.BumpLevel{"foo": semver.Minor},
		Body:  "Adds Lazy() helper.",
	}
	p := &plan.ReleasePlan{
		Releases: []plan.PackageRelease{{
			Name:       "foo",
			Config:     config.PackageConfig{TagPrefix: "transports/foo", Path: "transports/foo", Changelog: "transports/foo/CHANGELOG.md"},
			From:       "v1.6.1",
			To:         "v1.7.0",
			Bump:       semver.Minor,
			Tag:        "transports/foo/v1.7.0",
			Changesets: []*changeset.Changeset{cs1},
		}},
		Consumed: []*changeset.Changeset{cs1},
	}
	got := release.RenderPreview(p, "2026-04-30")

	for _, want := range []string{
		"opened by [monorel]",
		"`foo`",
		"v1.6.1",
		"**v1.7.0**",
		"### foo v1.7.0",
		"Adds Lazy() helper.",
		"`first`",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q\nfull:\n%s", want, got)
		}
	}
	// CHANGELOG H2 / H3 should have been bumped down so the page has
	// a clean H2 ("Released changes") + H3 (per-package) + H4
	// (per-bump-level) hierarchy.
	if strings.Contains(got, "\n## [1.7.0]") {
		t.Errorf("H2 from changelog leaked into preview body:\n%s", got)
	}
}

func TestRenderPreview_InitialFlagged(t *testing.T) {
	p := &plan.ReleasePlan{
		Releases: []plan.PackageRelease{{
			Name:    "new-pkg",
			Initial: true,
			From:    "",
			To:      "v0.1.0",
			Bump:    semver.Minor,
			Tag:     "new-pkg/v0.1.0",
		}},
	}
	got := release.RenderPreview(p, "2026-04-30")
	if !strings.Contains(got, "_(initial)_") {
		t.Errorf("initial release should render '(initial)' marker:\n%s", got)
	}
}

func TestRenderPreview_PrereleaseNote(t *testing.T) {
	p := &plan.ReleasePlan{
		Releases: []plan.PackageRelease{{
			Name:       "foo",
			From:       "v1.5.0",
			To:         "v1.6.0-rc.0",
			Bump:       semver.Minor,
			Tag:        "transports/foo/v1.6.0-rc.0",
			Prerelease: true,
		}},
	}
	got := release.RenderPreview(p, "2026-04-30")
	if !strings.Contains(got, "Pre-release") {
		t.Errorf("pre-release plan should call out the channel state:\n%s", got)
	}
}
