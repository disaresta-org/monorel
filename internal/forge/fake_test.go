package forge_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/disaresta-org/monorel/internal/forge"
)

func TestFake_FindOpenReleasePR_None(t *testing.T) {
	f := forge.NewFake()
	got, err := f.FindOpenReleasePR(context.Background(), "monorel/release")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("expected nil PR, got %+v", got)
	}
}

func TestFake_PRLifecycle(t *testing.T) {
	f := forge.NewFake()
	ctx := context.Background()

	pr, err := f.CreatePR(ctx, forge.CreatePROptions{
		Title:      "chore(release): publish",
		Body:       "first body",
		HeadBranch: "monorel/release",
		BaseBranch: "main",
	})
	if err != nil {
		t.Fatal(err)
	}
	if pr.Number != 1 {
		t.Errorf("first PR number = %d, want 1", pr.Number)
	}
	if pr.State != "open" {
		t.Errorf("State = %q, want open", pr.State)
	}

	found, err := f.FindOpenReleasePR(ctx, "monorel/release")
	if err != nil {
		t.Fatal(err)
	}
	if found == nil || found.Number != 1 {
		t.Errorf("FindOpenReleasePR returned %+v, want PR #1", found)
	}

	newBody := "updated body"
	updated, err := f.UpdatePR(ctx, 1, forge.UpdatePROptions{Body: &newBody})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Body != "updated body" {
		t.Errorf("Body = %q, want updated", updated.Body)
	}
	if updated.Title != "chore(release): publish" {
		t.Errorf("Title got mutated: %q", updated.Title)
	}

	if err := f.ClosePR(ctx, 1); err != nil {
		t.Fatal(err)
	}
	got, err := f.FindOpenReleasePR(ctx, "monorel/release")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("closed PR still appearing as open: %+v", got)
	}
}

func TestFake_CreatePR_Validates(t *testing.T) {
	f := forge.NewFake()
	ctx := context.Background()

	if _, err := f.CreatePR(ctx, forge.CreatePROptions{}); err == nil {
		t.Error("expected error for empty options")
	}
	if _, err := f.CreatePR(ctx, forge.CreatePROptions{Title: "ok", HeadBranch: "h"}); err == nil {
		t.Error("expected error for missing BaseBranch")
	}
}

func TestFake_UpdatePR_NotFound(t *testing.T) {
	f := forge.NewFake()
	body := "x"
	_, err := f.UpdatePR(context.Background(), 999, forge.UpdatePROptions{Body: &body})
	if err == nil {
		t.Fatal("expected error for missing PR")
	}
	if !strings.Contains(err.Error(), "999") {
		t.Errorf("error %q should mention PR number", err)
	}
}

func TestFake_CreateRelease(t *testing.T) {
	f := forge.NewFake()
	ctx := context.Background()
	r, err := f.CreateRelease(ctx, forge.CreateReleaseOptions{
		Tag:        "transports/foo/v1.6.0",
		Name:       "transports/foo v1.6.0",
		Body:       "## Minor Changes\n- Feature.",
		Prerelease: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if r.ID != 1 {
		t.Errorf("ID = %d, want 1", r.ID)
	}
	if r.Tag != "transports/foo/v1.6.0" {
		t.Errorf("Tag = %q", r.Tag)
	}
	if !strings.Contains(r.HTMLURL, "transports/foo/v1.6.0") {
		t.Errorf("HTMLURL = %q", r.HTMLURL)
	}
}

func TestFake_FailNext(t *testing.T) {
	f := forge.NewFake()
	want := errors.New("synthetic")
	f.FailNext = want

	if _, err := f.GetDefaultBranch(context.Background()); !errors.Is(err, want) {
		t.Errorf("err = %v, want %v", err, want)
	}
	if _, err := f.GetDefaultBranch(context.Background()); err != nil {
		t.Errorf("FailNext should be one-shot, got err: %v", err)
	}
}
