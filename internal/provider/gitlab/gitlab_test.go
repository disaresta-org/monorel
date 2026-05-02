package gitlab_test

import (
	"context"
	"errors"
	"testing"

	"monorel.disaresta.com/internal/provider/gitlab"
)

func TestNew_RejectsMissingOwnerRepo(t *testing.T) {
	cases := []struct {
		name string
		opts gitlab.Options
	}{
		{"no owner", gitlab.Options{Repo: "r", Token: "t"}},
		{"no repo", gitlab.Options{Owner: "o", Token: "t"}},
		{"neither", gitlab.Options{Token: "t"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := gitlab.New(context.Background(), tc.opts)
			if !errors.Is(err, gitlab.ErrMissingOwnerRepo) {
				t.Errorf("err = %v, want ErrMissingOwnerRepo", err)
			}
		})
	}
}

func TestNew_RejectsMissingToken(t *testing.T) {
	_, err := gitlab.New(context.Background(), gitlab.Options{Owner: "o", Repo: "r"})
	if !errors.Is(err, gitlab.ErrMissingToken) {
		t.Errorf("err = %v, want ErrMissingToken", err)
	}
}

func TestNew_AcceptsSubgroupOwner(t *testing.T) {
	// Owner can contain slashes for nested sub-groups. The
	// constructor doesn't validate the structure; the SDK's first
	// API call is what would error against an unreachable instance.
	// Here we only verify the constructor accepts it.
	_, err := gitlab.New(context.Background(), gitlab.Options{
		Owner: "team/platform",
		Repo:  "widget",
		Token: "test-token",
		Host:  "https://gitlab.example.com",
	})
	if err != nil {
		t.Errorf("constructor rejected sub-group owner: %v", err)
	}
}
