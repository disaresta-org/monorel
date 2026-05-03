package detect

import (
	"context"
	"errors"
	"strings"
	"testing"

	"monorel.disaresta.com/internal/git"
	"monorel.disaresta.com/internal/provider"
)

// stageAndCommit is a helper for the trivially-staged path the fake
// requires (Commit errors when nothing has been staged). The contents
// of the staged path are irrelevant; the test reads back HEAD's
// commit message via HeadCommitMessage.
func stageAndCommit(t *testing.T, repo *git.Fake, message string) {
	t.Helper()
	if err := repo.Add("dummy"); err != nil {
		t.Fatal(err)
	}
	if err := repo.Commit(message); err != nil {
		t.Fatal(err)
	}
}

func TestIsReleaseMerge_TrailerHits(t *testing.T) {
	repo := git.NewFake()
	stageAndCommit(t, repo, "chore(release): pkg-a v1.0.0\n\nmonorel-Release: pkg-a v1.0.0\n")
	pf := provider.NewFake()

	res, err := IsReleaseMerge(context.Background(), repo, pf, "")
	if err != nil {
		t.Fatalf("IsReleaseMerge: %v", err)
	}
	if !res.IsRelease {
		t.Errorf("IsRelease = false, want true")
	}
	if res.Source != SourceTrailer {
		t.Errorf("Source = %q, want %q", res.Source, SourceTrailer)
	}
	// Trailer signal short-circuits: API should not be called.
	// provider.Fake doesn't track call counts directly, but we can
	// prove the API was not invoked by setting FailNext: if the API
	// call had happened, the test would have failed with that error.
	// (Re-run a fresh call with FailNext set to confirm.)
	pf.FailNext = provider.FailOnce(errors.New("API should not have been called"))
	res2, err := IsReleaseMerge(context.Background(), repo, pf, "")
	if err != nil {
		t.Errorf("trailer fast path called the API: %v", err)
	}
	if res2 == nil || !res2.IsRelease || res2.Source != SourceTrailer {
		t.Errorf("trailer-only second call returned unexpected result: %+v", res2)
	}
}

func TestIsReleaseMerge_APIHits(t *testing.T) {
	repo := git.NewFake()
	// HEAD has no trailer.
	stageAndCommit(t, repo, "Merge pull request #5 from monorel/release\n")
	sha, _ := repo.CurrentSHA()

	pf := provider.NewFake()
	pr := &provider.PullRequest{
		Number:    5,
		State:     "closed",
		HeadRef:   "monorel/release",
		MergedSHA: sha,
	}
	pf.PRs[5] = pr

	res, err := IsReleaseMerge(context.Background(), repo, pf, sha)
	if err != nil {
		t.Fatalf("IsReleaseMerge: %v", err)
	}
	if !res.IsRelease {
		t.Errorf("IsRelease = false, want true")
	}
	if res.Source != SourceAPI {
		t.Errorf("Source = %q, want %q", res.Source, SourceAPI)
	}
	if res.PR == nil || res.PR.Number != 5 {
		t.Errorf("PR = %+v, want PR #5", res.PR)
	}
}

func TestIsReleaseMerge_APIReturnsWrongHeadRef(t *testing.T) {
	repo := git.NewFake()
	stageAndCommit(t, repo, "Merge pull request #5\n")
	sha, _ := repo.CurrentSHA()

	pf := provider.NewFake()
	pf.PRs[5] = &provider.PullRequest{
		Number:    5,
		State:     "closed",
		HeadRef:   "feature/something-else", // not monorel/release
		MergedSHA: sha,
	}

	res, err := IsReleaseMerge(context.Background(), repo, pf, sha)
	if err != nil {
		t.Fatal(err)
	}
	if res.IsRelease {
		t.Errorf("IsRelease = true, want false")
	}
	if res.Source != SourceNone {
		t.Errorf("Source = %q, want %q", res.Source, SourceNone)
	}
}

func TestIsReleaseMerge_APIReturnsNoPR(t *testing.T) {
	repo := git.NewFake()
	stageAndCommit(t, repo, "Some unrelated commit\n")
	pf := provider.NewFake() // no PRs seeded

	res, err := IsReleaseMerge(context.Background(), repo, pf, "deadbeef")
	if err != nil {
		t.Fatal(err)
	}
	if res.IsRelease {
		t.Errorf("IsRelease = true, want false")
	}
}

func TestIsReleaseMerge_APIErrorPropagates(t *testing.T) {
	repo := git.NewFake()
	stageAndCommit(t, repo, "Some commit\n")
	pf := provider.NewFake()
	pf.FailNext = provider.FailOnce(errors.New("network down"))

	_, err := IsReleaseMerge(context.Background(), repo, pf, "abcd")
	if err == nil {
		t.Fatal("expected wrapped network error")
	}
	// The error is wrapped, not the same value, so check substring.
	if !strings.Contains(err.Error(), "network down") {
		t.Errorf("err = %v; should wrap underlying network error", err)
	}
}

func TestIsReleaseMerge_RepoErrorPropagates(t *testing.T) {
	repo := git.NewFake()
	stageAndCommit(t, repo, "some commit\n")
	// Arm the next call to fail. git.Fake.FailNext is a plain error
	// (single-shot via take()); no FailOnce helper exists in the git
	// package. The next operation IsReleaseMerge performs is
	// HeadCommitMessage, so the error wrap path is exercised.
	repo.FailNext = errors.New("git ls-files exploded")

	pf := provider.NewFake()

	_, err := IsReleaseMerge(context.Background(), repo, pf, "abcd")
	if err == nil {
		t.Fatal("expected wrapped repo error")
	}
	if !strings.Contains(err.Error(), "git ls-files exploded") {
		t.Errorf("err = %v; should wrap underlying repo error", err)
	}
	if !strings.Contains(err.Error(), "read HEAD commit message") {
		t.Errorf("err = %v; should mention 'read HEAD commit message'", err)
	}
}

func TestIsReleaseMerge_NilProvider(t *testing.T) {
	repo := git.NewFake()
	_, err := IsReleaseMerge(context.Background(), repo, nil, "abcd")
	if !errors.Is(err, ErrProviderRequired) {
		t.Errorf("err = %v, want ErrProviderRequired", err)
	}
}

func TestIsReleaseMerge_NilRepo(t *testing.T) {
	pf := provider.NewFake()
	_, err := IsReleaseMerge(context.Background(), nil, pf, "abcd")
	if !errors.Is(err, ErrRepoRequired) {
		t.Errorf("err = %v, want ErrRepoRequired", err)
	}
}
