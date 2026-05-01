package plan_test

import (
	"strings"
	"testing"

	"github.com/disaresta-org/monorel/internal/changeset"
	"github.com/disaresta-org/monorel/internal/plan"
	"github.com/disaresta-org/monorel/internal/semver"
)

func TestPlan_Prerelease_FirstPre(t *testing.T) {
	c := cfg(map[string]string{"foo": "transports/foo"})
	pre := &changeset.PreState{Mode: "pre", Channel: "rc", Counters: map[string]int{}}
	p, err := plan.Plan(c,
		[]*changeset.Changeset{cs("c", map[string]semver.BumpLevel{"foo": semver.Minor})},
		[]string{"transports/foo/v1.5.0"},
		pre,
	)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(p.Releases) != 1 {
		t.Fatalf("Releases len = %d, want 1", len(p.Releases))
	}
	r := p.Releases[0]
	if r.From != "v1.5.0" {
		t.Errorf("From = %q, want v1.5.0", r.From)
	}
	if r.To != "v1.6.0-rc.0" {
		t.Errorf("To = %q, want v1.6.0-rc.0", r.To)
	}
	if !r.Prerelease {
		t.Error("Prerelease = false, want true")
	}
	if r.PrereleaseCounter != 0 {
		t.Errorf("PrereleaseCounter = %d, want 0", r.PrereleaseCounter)
	}
	if r.Tag != "transports/foo/v1.6.0-rc.0" {
		t.Errorf("Tag = %q, want transports/foo/v1.6.0-rc.0", r.Tag)
	}
}

func TestPlan_Prerelease_CounterFromState(t *testing.T) {
	c := cfg(map[string]string{"foo": "transports/foo"})
	pre := &changeset.PreState{
		Mode:     "pre",
		Channel:  "beta",
		Counters: map[string]int{"foo": 3},
	}
	p, err := plan.Plan(c,
		[]*changeset.Changeset{cs("c", map[string]semver.BumpLevel{"foo": semver.Major})},
		[]string{"transports/foo/v1.5.0"},
		pre,
	)
	if err != nil {
		t.Fatal(err)
	}
	r := p.Releases[0]
	if r.To != "v2.0.0-beta.3" {
		t.Errorf("To = %q, want v2.0.0-beta.3", r.To)
	}
	if r.PrereleaseCounter != 3 {
		t.Errorf("PrereleaseCounter = %d, want 3", r.PrereleaseCounter)
	}
}

func TestPlan_Prerelease_DoesNotMutateState(t *testing.T) {
	// The planner reads counters but does not mutate them. The
	// release applier increments them after a successful release.
	pre := &changeset.PreState{
		Mode:     "pre",
		Channel:  "rc",
		Counters: map[string]int{"foo": 5},
	}
	c := cfg(map[string]string{"foo": "transports/foo"})
	if _, err := plan.Plan(c,
		[]*changeset.Changeset{cs("c", map[string]semver.BumpLevel{"foo": semver.Patch})},
		[]string{"transports/foo/v1.0.0"},
		pre,
	); err != nil {
		t.Fatal(err)
	}
	if got := pre.Counters["foo"]; got != 5 {
		t.Errorf("planner mutated counter: got %d, want 5", got)
	}
}

func TestPlan_Prerelease_StableTagPicked_PreTagIgnored(t *testing.T) {
	// Existing pre-release tags must NOT count as the latest stable
	// version. From should be v1.5.0, not v2.0.0-rc.0.
	c := cfg(map[string]string{"foo": "transports/foo"})
	pre := &changeset.PreState{Mode: "pre", Channel: "rc"}
	p, err := plan.Plan(c,
		[]*changeset.Changeset{cs("c", map[string]semver.BumpLevel{"foo": semver.Minor})},
		[]string{
			"transports/foo/v1.5.0",
			"transports/foo/v2.0.0-rc.0",
		},
		pre,
	)
	if err != nil {
		t.Fatal(err)
	}
	r := p.Releases[0]
	if r.From != "v1.5.0" {
		t.Errorf("From = %q, want v1.5.0 (pre-release tags must not be picked as stable)", r.From)
	}
	if !strings.HasSuffix(r.To, "-rc.0") {
		t.Errorf("To = %q, want suffix -rc.0", r.To)
	}
}

func TestPlan_Prerelease_InitialRelease(t *testing.T) {
	// First-ever release for a package while in pre-release mode:
	// initial-from-bump computes v1.0.0 / v0.1.0 / v0.0.1, then the
	// suffix is appended.
	c := cfg(map[string]string{"new": "plugins/new"})
	pre := &changeset.PreState{Mode: "pre", Channel: "alpha"}
	p, err := plan.Plan(c,
		[]*changeset.Changeset{cs("first", map[string]semver.BumpLevel{"new": semver.Major})},
		nil,
		pre,
	)
	if err != nil {
		t.Fatal(err)
	}
	r := p.Releases[0]
	if r.To != "v1.0.0-alpha.0" {
		t.Errorf("To = %q, want v1.0.0-alpha.0", r.To)
	}
	if !r.Initial {
		t.Error("Initial = false, want true")
	}
	if !r.Prerelease {
		t.Error("Prerelease = false, want true")
	}
}
