package github_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	gh "github.com/disaresta-org/monorel/internal/github"
	"github.com/disaresta-org/monorel/internal/plan"
	"github.com/disaresta-org/monorel/internal/release"
)

func TestPublishReleases_AllTags(t *testing.T) {
	f := gh.NewFake()
	p := &plan.ReleasePlan{
		Releases: []plan.PackageRelease{
			{Name: "foo", Tag: "transports/foo/v1.6.0", To: "v1.6.0"},
			{Name: "bar", Tag: "transports/bar/v0.5.3", To: "v0.5.3"},
		},
	}
	res := &release.Result{
		Tags: []string{"transports/foo/v1.6.0", "transports/bar/v0.5.3"},
		Bodies: map[string]string{
			"transports/foo/v1.6.0": "## [1.6.0]\n### Minor Changes\n- Feature.",
			"transports/bar/v0.5.3": "## [0.5.3]\n### Patch Changes\n- Fix.",
		},
	}

	out, err := gh.PublishReleases(context.Background(), f, res, p)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 {
		t.Errorf("got %d releases, want 2", len(out))
	}
	if len(f.Releases) != 2 {
		t.Errorf("fake has %d releases, want 2", len(f.Releases))
	}
}

func TestPublishReleases_PrereleaseFlag(t *testing.T) {
	f := gh.NewFake()
	p := &plan.ReleasePlan{
		Releases: []plan.PackageRelease{
			{Name: "foo", Tag: "transports/foo/v1.6.0-rc.0", To: "v1.6.0-rc.0", Prerelease: true},
		},
	}
	res := &release.Result{
		Tags:   []string{"transports/foo/v1.6.0-rc.0"},
		Bodies: map[string]string{"transports/foo/v1.6.0-rc.0": "rc body"},
	}
	if _, err := gh.PublishReleases(context.Background(), f, res, p); err != nil {
		t.Fatal(err)
	}
	if len(f.Releases) != 1 {
		t.Fatalf("got %d releases", len(f.Releases))
	}
}

func TestPublishReleases_PartialOnError(t *testing.T) {
	f := gh.NewFake()
	p := &plan.ReleasePlan{
		Releases: []plan.PackageRelease{
			{Name: "foo", Tag: "transports/foo/v1.6.0", To: "v1.6.0"},
			{Name: "bar", Tag: "transports/bar/v0.5.3", To: "v0.5.3"},
		},
	}
	res := &release.Result{
		Bodies: map[string]string{
			"transports/foo/v1.6.0": "ok",
			"transports/bar/v0.5.3": "ok",
		},
	}

	// Set FailNext to fail the SECOND CreateRelease (bar).
	// We achieve this by publishing one release manually first to
	// "consume" the natural flow, then setting FailNext.
	if _, err := f.CreateRelease(context.Background(), gh.CreateReleaseOptions{Tag: "warmup"}); err != nil {
		t.Fatal(err)
	}
	// Reset releases so the test cleanly counts.
	f.Releases = make(map[int64]*gh.Release)

	// Now FailNext fires on the FIRST publish call.
	wantErr := errors.New("synthetic")
	f.FailNext = wantErr

	out, err := gh.PublishReleases(context.Background(), f, res, p)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "synthetic") {
		t.Errorf("error %q should contain synthetic", err)
	}
	// The first publish failed; second wasn't attempted. out is empty.
	if len(out) != 0 {
		t.Errorf("partial out len = %d, want 0", len(out))
	}
}

func TestPublishReleases_NilArgs(t *testing.T) {
	if _, err := gh.PublishReleases(context.Background(), gh.NewFake(), nil, &plan.ReleasePlan{}); err == nil {
		t.Error("expected error for nil result")
	}
	if _, err := gh.PublishReleases(context.Background(), gh.NewFake(), &release.Result{}, nil); err == nil {
		t.Error("expected error for nil plan")
	}
}
