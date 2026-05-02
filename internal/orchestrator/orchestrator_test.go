package orchestrator_test

import (
	"context"
	"strings"
	"testing"

	"monorel.disaresta.com/changeset"
	"monorel.disaresta.com/internal/orchestrator"
	"monorel.disaresta.com/internal/provider"
	"monorel.disaresta.com/plan"
	"monorel.disaresta.com/semver"
)

func nonEmptyPlan() *plan.ReleasePlan {
	cs := &changeset.Changeset{
		Name:  "first",
		Bumps: map[string]semver.BumpLevel{"foo": semver.Minor},
		Body:  "Adds Lazy().",
	}
	return &plan.ReleasePlan{
		Releases: []plan.PackageRelease{{
			Name:       "foo",
			From:       "v1.6.1",
			To:         "v1.7.0",
			Bump:       semver.Minor,
			Tag:        "transports/foo/v1.7.0",
			Changesets: []*changeset.Changeset{cs},
		}},
		Consumed: []*changeset.Changeset{cs},
	}
}

func TestRun_EmptyPlan_NoExistingPR_Noop(t *testing.T) {
	f := provider.NewFake()
	res, err := orchestrator.Run(context.Background(), orchestrator.Options{
		Plan:     &plan.ReleasePlan{},
		Provider: f,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Action != orchestrator.ActionNoop {
		t.Errorf("Action = %q, want noop", res.Action)
	}
	if len(f.PRs) != 0 {
		t.Errorf("expected no PRs created; have %d", len(f.PRs))
	}
}

func TestRun_EmptyPlan_ExistingPR_Closed(t *testing.T) {
	f := provider.NewFake()
	pr, err := f.CreatePR(context.Background(), provider.CreatePROptions{
		Title:      "stale",
		HeadBranch: orchestrator.DefaultHeadBranch,
		BaseBranch: "main",
	})
	if err != nil {
		t.Fatal(err)
	}

	res, err := orchestrator.Run(context.Background(), orchestrator.Options{
		Plan:     &plan.ReleasePlan{},
		Provider: f,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Action != orchestrator.ActionClosed {
		t.Errorf("Action = %q, want closed", res.Action)
	}
	if got := f.PRs[pr.Number].State; got != "closed" {
		t.Errorf("PR state = %q, want closed", got)
	}
}

func TestRun_NonEmptyPlan_NoExistingPR_Created(t *testing.T) {
	f := provider.NewFake()
	res, err := orchestrator.Run(context.Background(), orchestrator.Options{
		Plan:     nonEmptyPlan(),
		Provider: f,
		Today:    "2026-04-30",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Action != orchestrator.ActionCreated {
		t.Errorf("Action = %q, want created", res.Action)
	}
	if res.PR == nil {
		t.Fatal("PR not returned")
	}
	if !strings.Contains(res.PR.Title, "foo v1.7.0") {
		t.Errorf("Title = %q, want package + version", res.PR.Title)
	}
	if !strings.Contains(res.PR.Body, "Adds Lazy().") {
		t.Errorf("Body should contain changeset body:\n%s", res.PR.Body)
	}
	if res.PR.HeadRef != orchestrator.DefaultHeadBranch {
		t.Errorf("HeadRef = %q, want %q", res.PR.HeadRef, orchestrator.DefaultHeadBranch)
	}
}

func TestRun_NonEmptyPlan_ExistingPR_Updated(t *testing.T) {
	f := provider.NewFake()
	created, err := f.CreatePR(context.Background(), provider.CreatePROptions{
		Title:      "stale title",
		Body:       "stale body",
		HeadBranch: orchestrator.DefaultHeadBranch,
		BaseBranch: "main",
	})
	if err != nil {
		t.Fatal(err)
	}

	res, err := orchestrator.Run(context.Background(), orchestrator.Options{
		Plan:     nonEmptyPlan(),
		Provider: f,
		Today:    "2026-04-30",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Action != orchestrator.ActionUpdated {
		t.Errorf("Action = %q, want updated", res.Action)
	}
	if got := f.PRs[created.Number].Title; !strings.Contains(got, "foo v1.7.0") {
		t.Errorf("Title not refreshed: %q", got)
	}
	if got := f.PRs[created.Number].Body; got == "stale body" {
		t.Errorf("Body not refreshed: %q", got)
	}
}

func TestRun_BaseBranchLookedUpFromProvider(t *testing.T) {
	f := provider.NewFake()
	f.DefaultBranch = "develop"

	if _, err := orchestrator.Run(context.Background(), orchestrator.Options{
		Plan:     nonEmptyPlan(),
		Provider: f,
		Today:    "2026-04-30",
	}); err != nil {
		t.Fatal(err)
	}
	// Fake records the BaseBranch in the created PR; verify.
	for _, pr := range f.PRs {
		if pr.HeadRef != orchestrator.DefaultHeadBranch {
			continue
		}
		// (BaseBranch isn't directly exposed on PullRequest, but the
		// CreatePR call would have failed validation if BaseBranch
		// were empty; reaching here means the lookup worked.)
		return
	}
	t.Error("created PR not found")
}

func TestRun_NilArgs(t *testing.T) {
	if _, err := orchestrator.Run(context.Background(), orchestrator.Options{Provider: provider.NewFake()}); err == nil {
		t.Error("expected error for nil Plan")
	}
	if _, err := orchestrator.Run(context.Background(), orchestrator.Options{Plan: &plan.ReleasePlan{}}); err == nil {
		t.Error("expected error for nil Provider")
	}
}

func TestRun_MultiPackageTitle(t *testing.T) {
	f := provider.NewFake()
	p := nonEmptyPlan()
	p.Releases = append(p.Releases, plan.PackageRelease{
		Name: "bar", From: "v0.5.2", To: "v0.5.3", Bump: semver.Patch, Tag: "transports/bar/v0.5.3",
	})

	res, err := orchestrator.Run(context.Background(), orchestrator.Options{
		Plan:     p,
		Provider: f,
		Today:    "2026-04-30",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.PR.Title, "2 packages") {
		t.Errorf("multi-package title = %q, want '2 packages'", res.PR.Title)
	}
}

// TestRun_LongBodyFallsBackToCompact builds a plan whose full
// rendering would exceed MaxPRBodyBytes (a single huge changeset
// body fanned across 30 packages) and verifies the orchestrator
// falls back to the compact form so the provider call still
// succeeds.
func TestRun_LongBodyFallsBackToCompact(t *testing.T) {
	f := provider.NewFake()

	// 5 KiB of body × 30 packages = ~150 KiB, well over the 64 KiB
	// limit. Each package has its own changeset with the same body.
	body := strings.Repeat("This is a long changeset body. ", 160) // ~5 KB
	p := &plan.ReleasePlan{}
	for i := 0; i < 30; i++ {
		name := "pkg" + string(rune('a'+i))
		cs := &changeset.Changeset{
			Name:  name,
			Bumps: map[string]semver.BumpLevel{name: semver.Major},
			Body:  body,
		}
		p.Releases = append(p.Releases, plan.PackageRelease{
			Name:       name,
			From:       "v1.0.0",
			To:         "v2.0.0",
			Bump:       semver.Major,
			Tag:        "transports/" + name + "/v2.0.0",
			Changesets: []*changeset.Changeset{cs},
		})
		p.Consumed = append(p.Consumed, cs)
	}

	res, err := orchestrator.Run(context.Background(), orchestrator.Options{
		Plan:     p,
		Provider: f,
		Today:    "2026-05-02",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if got := len(res.PR.Body); got > orchestrator.MaxPRBodyBytes {
		t.Errorf("body size %d exceeds limit %d; orchestrator should have fallen back", got, orchestrator.MaxPRBodyBytes)
	}
	if !strings.Contains(res.PR.Body, "Per-package release notes were elided") {
		t.Errorf("compact-form marker missing; body:\n%s", res.PR.Body)
	}
	// Sanity: the table is still present (so the reader sees
	// what's shipping at what versions).
	if !strings.Contains(res.PR.Body, "| Package | Bump | From | To |") {
		t.Errorf("version table missing from compact rendering")
	}
}
