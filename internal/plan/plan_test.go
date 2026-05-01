package plan_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/disaresta-org/monorel/internal/changeset"
	"github.com/disaresta-org/monorel/internal/config"
	"github.com/disaresta-org/monorel/internal/plan"
	"github.com/disaresta-org/monorel/internal/semver"
)

// cfg builds a Config from package-name -> tag_prefix pairs. owner/repo
// are seeded so Validate would pass; the planner doesn't actually call
// Validate but we mimic a real-shaped input.
func cfg(prefixes map[string]string) *config.Config {
	pkgs := make(map[string]config.PackageConfig, len(prefixes))
	for name, prefix := range prefixes {
		pkgs[name] = config.PackageConfig{
			TagPrefix: prefix,
			Path:      strings.TrimPrefix(name, "github.com/x/"),
			Changelog: name + "/CHANGELOG.md",
		}
	}
	return &config.Config{
		GitHub:   config.GitHubConfig{Owner: "x", Repo: "y"},
		Packages: pkgs,
	}
}

// cs builds a single changeset with the given name and bumps.
func cs(name string, bumps map[string]semver.BumpLevel) *changeset.Changeset {
	return &changeset.Changeset{Name: name, Bumps: bumps, Body: name + " body"}
}

func TestPlan_SinglePackage_BumpLevels(t *testing.T) {
	tests := []struct {
		name  string
		level semver.BumpLevel
		want  string
	}{
		{"patch", semver.Patch, "v1.6.2"},
		{"minor", semver.Minor, "v1.7.0"},
		{"major", semver.Major, "v2.0.0"},
	}
	c := cfg(map[string]string{"foo": "transports/foo"})
	tags := []string{"transports/foo/v1.6.1", "transports/foo/v1.5.0", "transports/foo/v1.6.0"}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := plan.Plan(c, []*changeset.Changeset{
				cs("a", map[string]semver.BumpLevel{"foo": tt.level}),
			}, tags, nil)
			if err != nil {
				t.Fatalf("Plan: %v", err)
			}
			if len(p.Releases) != 1 {
				t.Fatalf("Releases len = %d, want 1", len(p.Releases))
			}
			r := p.Releases[0]
			if r.Name != "foo" || r.From != "v1.6.1" || r.To != tt.want {
				t.Errorf("got Name=%q From=%q To=%q, want foo / v1.6.1 / %s", r.Name, r.From, r.To, tt.want)
			}
			if r.Bump != tt.level {
				t.Errorf("Bump = %v, want %v", r.Bump, tt.level)
			}
			if r.Tag != "transports/foo/"+tt.want {
				t.Errorf("Tag = %q, want transports/foo/%s", r.Tag, tt.want)
			}
			if r.Initial {
				t.Error("Initial = true, want false (existing tag found)")
			}
		})
	}
}

func TestPlan_MultiChangeset_MaxBump(t *testing.T) {
	c := cfg(map[string]string{"foo": "transports/foo"})
	tags := []string{"transports/foo/v1.0.0"}

	// Three changesets: patch, minor, major. Major wins.
	p, err := plan.Plan(c, []*changeset.Changeset{
		cs("c1", map[string]semver.BumpLevel{"foo": semver.Patch}),
		cs("c2", map[string]semver.BumpLevel{"foo": semver.Minor}),
		cs("c3", map[string]semver.BumpLevel{"foo": semver.Major}),
	}, tags, nil)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(p.Releases) != 1 {
		t.Fatalf("Releases len = %d, want 1", len(p.Releases))
	}
	if got := p.Releases[0].Bump; got != semver.Major {
		t.Errorf("Bump = %v, want Major", got)
	}
	if got := p.Releases[0].To; got != "v2.0.0" {
		t.Errorf("To = %q, want v2.0.0", got)
	}
	// All three changesets should be attached to the foo release in
	// sorted order, AND all three should be in Consumed.
	if got := len(p.Releases[0].Changesets); got != 3 {
		t.Errorf("len(Changesets) = %d, want 3", got)
	}
	if got := p.Releases[0].Changesets[0].Name; got != "c1" {
		t.Errorf("Changesets[0].Name = %q, want c1", got)
	}
	if got := len(p.Consumed); got != 3 {
		t.Errorf("len(Consumed) = %d, want 3", got)
	}
}

func TestPlan_MultiPackageChangeset(t *testing.T) {
	c := cfg(map[string]string{
		"foo": "transports/foo",
		"bar": "transports/bar",
	})
	tags := []string{
		"transports/foo/v1.0.0",
		"transports/bar/v0.5.2",
	}

	// One changeset that bumps both packages with different levels.
	p, err := plan.Plan(c, []*changeset.Changeset{
		cs("hot-cat", map[string]semver.BumpLevel{
			"foo": semver.Major,
			"bar": semver.Patch,
		}),
	}, tags, nil)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(p.Releases) != 2 {
		t.Fatalf("Releases len = %d, want 2", len(p.Releases))
	}
	// Sorted by name: bar before foo.
	if p.Releases[0].Name != "bar" || p.Releases[0].To != "v0.5.3" {
		t.Errorf("bar release: Name=%q To=%q, want bar / v0.5.3", p.Releases[0].Name, p.Releases[0].To)
	}
	if p.Releases[1].Name != "foo" || p.Releases[1].To != "v2.0.0" {
		t.Errorf("foo release: Name=%q To=%q, want foo / v2.0.0", p.Releases[1].Name, p.Releases[1].To)
	}
	// Consumed should have one entry (the single changeset).
	if got := len(p.Consumed); got != 1 {
		t.Errorf("len(Consumed) = %d, want 1", got)
	}
}

func TestPlan_InitialRelease(t *testing.T) {
	tests := []struct {
		name  string
		level semver.BumpLevel
		want  string
	}{
		{"major-yields-v1", semver.Major, "v1.0.0"},
		{"minor-yields-v0.1.0", semver.Minor, "v0.1.0"},
		{"patch-yields-v0.0.1", semver.Patch, "v0.0.1"},
	}
	c := cfg(map[string]string{"new": "plugins/new"})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// No tags exist for "plugins/new".
			p, err := plan.Plan(c, []*changeset.Changeset{
				cs("first", map[string]semver.BumpLevel{"new": tt.level}),
			}, []string{"unrelated/v1.0.0", "v9.9.9"}, nil)
			if err != nil {
				t.Fatalf("Plan: %v", err)
			}
			if len(p.Releases) != 1 {
				t.Fatalf("Releases len = %d, want 1", len(p.Releases))
			}
			r := p.Releases[0]
			if r.From != "" {
				t.Errorf("From = %q, want empty (no prior tag)", r.From)
			}
			if r.To != tt.want {
				t.Errorf("To = %q, want %q", r.To, tt.want)
			}
			if !r.Initial {
				t.Error("Initial = false, want true")
			}
			if r.Tag != "plugins/new/"+tt.want {
				t.Errorf("Tag = %q, want plugins/new/%s", r.Tag, tt.want)
			}
		})
	}
}

func TestPlan_UnknownPackage(t *testing.T) {
	c := cfg(map[string]string{"foo": "transports/foo"})
	_, err := plan.Plan(c, []*changeset.Changeset{
		cs("bad", map[string]semver.BumpLevel{"nonexistent": semver.Patch}),
	}, nil, nil)
	if err == nil {
		t.Fatal("expected error for unknown package; got nil")
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("error message should mention package name: %v", err)
	}
}

func TestPlan_PackageWithoutChangesetsNotInPlan(t *testing.T) {
	c := cfg(map[string]string{
		"foo": "transports/foo",
		"bar": "transports/bar",
		"baz": "transports/baz",
	})
	tags := []string{
		"transports/foo/v1.0.0",
		"transports/bar/v1.0.0",
		"transports/baz/v1.0.0",
	}
	// Only foo gets a changeset.
	p, err := plan.Plan(c, []*changeset.Changeset{
		cs("only-foo", map[string]semver.BumpLevel{"foo": semver.Patch}),
	}, tags, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Releases) != 1 {
		t.Fatalf("Releases len = %d, want 1: %+v", len(p.Releases), p.Releases)
	}
	if p.Releases[0].Name != "foo" {
		t.Errorf("got %q, want foo", p.Releases[0].Name)
	}
}

func TestPlan_NonSemverTagsIgnored(t *testing.T) {
	c := cfg(map[string]string{"foo": "transports/foo"})
	// The "v1.0.0-not-a-real-version" lookalike, the bare "v1.0", and
	// the non-leading-v "1.6.0" all get filtered out. v1.5.0 wins.
	tags := []string{
		"transports/foo/v1.5.0",
		"transports/foo/v1.0",
		"transports/foo/1.6.0",
		"transports/foo/initial-release",
	}
	p, err := plan.Plan(c, []*changeset.Changeset{
		cs("c", map[string]semver.BumpLevel{"foo": semver.Patch}),
	}, tags, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := p.Releases[0].From; got != "v1.5.0" {
		t.Errorf("From = %q, want v1.5.0", got)
	}
	if got := p.Releases[0].To; got != "v1.5.1" {
		t.Errorf("To = %q, want v1.5.1", got)
	}
}

func TestPlan_PrereleaseTagsIgnored(t *testing.T) {
	c := cfg(map[string]string{"foo": "transports/foo"})
	// The pre-release tag is ignored when finding the latest STABLE
	// version. The planner produces a v1.5.x bump, not a v2.0.0-rc.1
	// derivative.
	tags := []string{
		"transports/foo/v1.5.0",
		"transports/foo/v2.0.0-rc.1",
		"transports/foo/v2.0.0-rc.2",
	}
	p, err := plan.Plan(c, []*changeset.Changeset{
		cs("c", map[string]semver.BumpLevel{"foo": semver.Minor}),
	}, tags, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := p.Releases[0].From; got != "v1.5.0" {
		t.Errorf("From = %q, want v1.5.0 (pre-release tags should be ignored)", got)
	}
	if got := p.Releases[0].To; got != "v1.6.0" {
		t.Errorf("To = %q, want v1.6.0", got)
	}
}

func TestPlan_BareTagRoot(t *testing.T) {
	// Empty tag_prefix: tags like "v1.0.0" match, but "transports/foo/v1.0.0" must NOT.
	c := cfg(map[string]string{"core": ""})
	tags := []string{"v1.6.0", "v1.6.1", "transports/foo/v9.9.9"}
	p, err := plan.Plan(c, []*changeset.Changeset{
		cs("c", map[string]semver.BumpLevel{"core": semver.Minor}),
	}, tags, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := p.Releases[0].From; got != "v1.6.1" {
		t.Errorf("From = %q, want v1.6.1 (bare-tag root must not match prefixed tags)", got)
	}
	if got := p.Releases[0].To; got != "v1.7.0" {
		t.Errorf("To = %q, want v1.7.0", got)
	}
	if got := p.Releases[0].Tag; got != "v1.7.0" {
		t.Errorf("Tag = %q, want v1.7.0 (bare)", got)
	}
}

func TestPlan_EmptyChangesetsYieldsEmptyPlan(t *testing.T) {
	c := cfg(map[string]string{"foo": "transports/foo"})
	p, err := plan.Plan(c, nil, []string{"transports/foo/v1.0.0"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !p.IsEmpty() {
		t.Error("expected empty plan from empty changesets")
	}
}

func TestPlan_NilConfig(t *testing.T) {
	_, err := plan.Plan(nil, nil, nil, nil)
	if err == nil {
		t.Fatal("expected error for nil config")
	}
}

func TestPlan_DeterministicOrder(t *testing.T) {
	// Different input orderings must produce identical Releases and
	// Consumed slices.
	c := cfg(map[string]string{
		"foo": "transports/foo",
		"bar": "transports/bar",
		"baz": "transports/baz",
	})
	tags := []string{
		"transports/foo/v1.0.0",
		"transports/bar/v1.0.0",
		"transports/baz/v1.0.0",
	}
	a := []*changeset.Changeset{
		cs("z-last", map[string]semver.BumpLevel{"foo": semver.Patch}),
		cs("a-first", map[string]semver.BumpLevel{"bar": semver.Patch, "baz": semver.Patch}),
	}
	b := []*changeset.Changeset{
		cs("a-first", map[string]semver.BumpLevel{"baz": semver.Patch, "bar": semver.Patch}),
		cs("z-last", map[string]semver.BumpLevel{"foo": semver.Patch}),
	}
	pa, err := plan.Plan(c, a, tags, nil)
	if err != nil {
		t.Fatal(err)
	}
	pb, err := plan.Plan(c, b, tags, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(pa.Releases) != len(pb.Releases) {
		t.Fatalf("len mismatch: %d vs %d", len(pa.Releases), len(pb.Releases))
	}
	for i := range pa.Releases {
		if pa.Releases[i].Name != pb.Releases[i].Name {
			t.Errorf("Releases[%d].Name: %q vs %q", i, pa.Releases[i].Name, pb.Releases[i].Name)
		}
	}
	if len(pa.Consumed) != len(pb.Consumed) {
		t.Fatalf("Consumed len mismatch: %d vs %d", len(pa.Consumed), len(pb.Consumed))
	}
	for i := range pa.Consumed {
		if pa.Consumed[i].Name != pb.Consumed[i].Name {
			t.Errorf("Consumed[%d].Name: %q vs %q", i, pa.Consumed[i].Name, pb.Consumed[i].Name)
		}
	}
}

func TestPlan_SameChangesetCountedOnceInConsumed(t *testing.T) {
	// A changeset bumping multiple packages must appear once in
	// Consumed even though it appears in N PackageRelease.Changesets.
	c := cfg(map[string]string{
		"foo": "transports/foo",
		"bar": "transports/bar",
	})
	tags := []string{"transports/foo/v1.0.0", "transports/bar/v1.0.0"}
	p, err := plan.Plan(c, []*changeset.Changeset{
		cs("multi", map[string]semver.BumpLevel{
			"foo": semver.Patch,
			"bar": semver.Patch,
		}),
	}, tags, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Consumed) != 1 {
		t.Fatalf("len(Consumed) = %d, want 1", len(p.Consumed))
	}
	if p.Consumed[0].Name != "multi" {
		t.Errorf("Consumed[0].Name = %q, want multi", p.Consumed[0].Name)
	}
	// And the changeset should be in BOTH releases' Changesets.
	for _, r := range p.Releases {
		if len(r.Changesets) != 1 || r.Changesets[0].Name != "multi" {
			t.Errorf("%s.Changesets = %+v, want one entry named multi", r.Name, r.Changesets)
		}
	}
}

func TestPlan_BumpLevelMaxAcrossPackages(t *testing.T) {
	// foo gets two changesets (one Patch, one Major) — Major wins.
	// bar gets one Minor changeset.
	c := cfg(map[string]string{
		"foo": "transports/foo",
		"bar": "transports/bar",
	})
	tags := []string{"transports/foo/v1.6.1", "transports/bar/v2.0.0"}
	p, err := plan.Plan(c, []*changeset.Changeset{
		cs("a", map[string]semver.BumpLevel{"foo": semver.Patch}),
		cs("b", map[string]semver.BumpLevel{"foo": semver.Major, "bar": semver.Minor}),
	}, tags, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range p.Releases {
		switch r.Name {
		case "foo":
			if r.Bump != semver.Major || r.To != "v2.0.0" {
				t.Errorf("foo: Bump=%v To=%q, want Major / v2.0.0", r.Bump, r.To)
			}
		case "bar":
			if r.Bump != semver.Minor || r.To != "v2.1.0" {
				t.Errorf("bar: Bump=%v To=%q, want Minor / v2.1.0", r.Bump, r.To)
			}
		}
	}
}

// Sanity: error path returns wrapped error (used by upstream callers
// that want to display "changeset X has invalid package Y").
func TestPlan_ErrorIsHelpful(t *testing.T) {
	c := cfg(map[string]string{"foo": "transports/foo"})
	_, err := plan.Plan(c, []*changeset.Changeset{
		cs("messy", map[string]semver.BumpLevel{"foo": semver.Patch, "ghost": semver.Patch}),
	}, nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "messy") || !strings.Contains(err.Error(), "ghost") {
		t.Errorf("error %q should mention both changeset name and bad package", err)
	}
	// Make sure the error is not a generic non-wrap (sanity).
	if errors.Is(err, errors.New("unrelated")) {
		t.Error("errors.Is should not match unrelated sentinel")
	}
}
