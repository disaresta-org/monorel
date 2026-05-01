package release_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/disaresta-org/monorel/internal/forge"
	"github.com/disaresta-org/monorel/internal/release"
)

func TestPublishReleases_AllTags(t *testing.T) {
	f := forge.NewFake()
	res := &release.Result{
		Releases: []release.ReleaseInfo{
			{Tag: "transports/foo/v1.6.0", Body: "## [1.6.0]\n### Minor Changes\n- Feature."},
			{Tag: "transports/bar/v0.5.3", Body: "## [0.5.3]\n### Patch Changes\n- Fix."},
		},
	}

	out, err := release.PublishReleases(context.Background(), f, res)
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
	f := forge.NewFake()
	res := &release.Result{
		Releases: []release.ReleaseInfo{
			{Tag: "transports/foo/v1.6.0-rc.0", Body: "rc body", Prerelease: true},
		},
	}
	if _, err := release.PublishReleases(context.Background(), f, res); err != nil {
		t.Fatal(err)
	}
	if len(f.Releases) != 1 {
		t.Fatalf("got %d releases", len(f.Releases))
	}
}

func TestPublishReleases_PartialOnError(t *testing.T) {
	// Three releases; the second CreateRelease call fails. The
	// returned slice must contain the first (successful) release.
	f := forge.NewFake()
	wantErr := errors.New("synthetic")
	f.FailNext = forge.FailOnNth(2, wantErr)

	res := &release.Result{
		Releases: []release.ReleaseInfo{
			{Tag: "a", Body: "body a"},
			{Tag: "b", Body: "body b"},
			{Tag: "c", Body: "body c"},
		},
	}

	out, err := release.PublishReleases(context.Background(), f, res)
	if err == nil {
		t.Fatal("expected error from second CreateRelease")
	}
	if !strings.Contains(err.Error(), "synthetic") {
		t.Errorf("error %q should contain synthetic", err)
	}
	if len(out) != 1 {
		t.Errorf("partial out len = %d, want 1 (the first release succeeded)", len(out))
	}
	if len(out) > 0 && out[0].Tag != "a" {
		t.Errorf("partial out[0].Tag = %q, want a", out[0].Tag)
	}
	if len(f.Releases) != 1 {
		t.Errorf("fake has %d releases, want 1 (only 'a' was created)", len(f.Releases))
	}
}

func TestPublishReleases_NilResult(t *testing.T) {
	if _, err := release.PublishReleases(context.Background(), forge.NewFake(), nil); err == nil {
		t.Error("expected error for nil result")
	}
}
