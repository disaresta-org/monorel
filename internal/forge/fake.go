package forge

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// Fake is an in-memory [Client] for unit tests of the orchestrator
// and any forge-aware higher layer. Provider-neutral: same fake
// satisfies the contract for every forge implementation.
type Fake struct {
	mu sync.Mutex

	// DefaultBranch is what GetDefaultBranch returns. Default "main".
	DefaultBranch string

	// PRs is every PR ever created on this fake, keyed by number.
	// Numbers are auto-assigned starting at 1.
	PRs map[int]*PullRequest

	// Releases is every Release ever created, keyed by ID. IDs are
	// auto-assigned starting at 1.
	Releases map[int64]*Release

	// FailNext, when non-nil, is consulted before every API call.
	// Return non-nil to fail the call. The fake doesn't manage call
	// counts; tests own the closure state. Use [FailOnce] for a
	// one-shot failure or [FailOnNth] for "succeed N-1 times then
	// fail."
	FailNext func() error
}

// NewFake returns an empty Fake with DefaultBranch="main".
func NewFake() *Fake {
	return &Fake{
		DefaultBranch: "main",
		PRs:           map[int]*PullRequest{},
		Releases:      map[int64]*Release{},
	}
}

// FailOnce returns a closure that fails once with err and then
// succeeds. Use as [Fake.FailNext] for the common single-shot
// fault-injection pattern.
func FailOnce(err error) func() error {
	fired := false
	return func() error {
		if fired {
			return nil
		}
		fired = true
		return err
	}
}

// FailOnNth returns a closure that returns nil for the first n-1
// calls, fails with err on the nth, then succeeds. Use to test
// partial-success paths. n must be >= 1.
func FailOnNth(n int, err error) func() error {
	var calls int
	return func() error {
		calls++
		if calls == n {
			return err
		}
		return nil
	}
}

func (f *Fake) take() error {
	if f.FailNext == nil {
		return nil
	}
	return f.FailNext()
}

func (f *Fake) nextPRNumber() int {
	max := 0
	for n := range f.PRs {
		if n > max {
			max = n
		}
	}
	return max + 1
}

func (f *Fake) nextReleaseID() int64 {
	var max int64
	for id := range f.Releases {
		if id > max {
			max = id
		}
	}
	return max + 1
}

// GetDefaultBranch implements [Client.GetDefaultBranch].
func (f *Fake) GetDefaultBranch(_ context.Context) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.take(); err != nil {
		return "", err
	}
	return f.DefaultBranch, nil
}

// FindOpenReleasePR implements [Client.FindOpenReleasePR].
func (f *Fake) FindOpenReleasePR(_ context.Context, headBranch string) (*PullRequest, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.take(); err != nil {
		return nil, err
	}
	for _, pr := range f.PRs {
		if pr.State == "open" && pr.HeadRef == headBranch {
			cp := *pr
			return &cp, nil
		}
	}
	return nil, nil
}

// CreatePR implements [Client.CreatePR].
func (f *Fake) CreatePR(_ context.Context, opts CreatePROptions) (*PullRequest, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.take(); err != nil {
		return nil, err
	}
	if opts.Title == "" {
		return nil, errors.New("forge fake: CreatePR title is empty")
	}
	if opts.HeadBranch == "" || opts.BaseBranch == "" {
		return nil, errors.New("forge fake: CreatePR HeadBranch/BaseBranch required")
	}
	num := f.nextPRNumber()
	pr := &PullRequest{
		Number:  num,
		State:   "open",
		Title:   opts.Title,
		Body:    opts.Body,
		HeadRef: opts.HeadBranch,
		HTMLURL: fmt.Sprintf("https://forge.fake/fake/pull/%d", num),
	}
	f.PRs[num] = pr
	cp := *pr
	return &cp, nil
}

// UpdatePR implements [Client.UpdatePR]. Mirrors the GitHub impl by
// rejecting a no-op patch (both Title and Body nil); a no-op patch is
// a programmer error rather than a silent success.
func (f *Fake) UpdatePR(_ context.Context, number int, opts UpdatePROptions) (*PullRequest, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.take(); err != nil {
		return nil, err
	}
	if opts.Title == nil && opts.Body == nil {
		return nil, errors.New("forge fake: UpdatePR has nothing to change")
	}
	pr, ok := f.PRs[number]
	if !ok {
		return nil, fmt.Errorf("forge fake: PR #%d not found", number)
	}
	if opts.Title != nil {
		pr.Title = *opts.Title
	}
	if opts.Body != nil {
		pr.Body = *opts.Body
	}
	cp := *pr
	return &cp, nil
}

// ClosePR implements [Client.ClosePR].
func (f *Fake) ClosePR(_ context.Context, number int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.take(); err != nil {
		return err
	}
	pr, ok := f.PRs[number]
	if !ok {
		return fmt.Errorf("forge fake: PR #%d not found", number)
	}
	pr.State = "closed"
	return nil
}

// CreateRelease implements [Client.CreateRelease].
func (f *Fake) CreateRelease(_ context.Context, opts CreateReleaseOptions) (*Release, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.take(); err != nil {
		return nil, err
	}
	if opts.Tag == "" {
		return nil, errors.New("forge fake: CreateRelease Tag is empty")
	}
	id := f.nextReleaseID()
	rel := &Release{
		ID:      id,
		Tag:     opts.Tag,
		HTMLURL: fmt.Sprintf("https://forge.fake/fake/releases/tag/%s", opts.Tag),
	}
	f.Releases[id] = rel
	cp := *rel
	return &cp, nil
}
