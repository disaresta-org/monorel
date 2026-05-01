package gitea_test

import (
	"context"
	"errors"
	"testing"

	"monorel.disaresta.com/internal/provider/gitea"
)

func TestNew_RejectsMissingOwnerRepo(t *testing.T) {
	cases := []struct {
		name string
		opts gitea.Options
	}{
		{"no owner", gitea.Options{Repo: "r", Host: "gitea.example.com"}},
		{"no repo", gitea.Options{Owner: "o", Host: "gitea.example.com"}},
		{"neither", gitea.Options{Host: "gitea.example.com"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := gitea.New(context.Background(), tc.opts)
			if !errors.Is(err, gitea.ErrMissingOwnerRepo) {
				t.Errorf("err = %v, want ErrMissingOwnerRepo", err)
			}
		})
	}
}

func TestNew_RejectsMissingHost(t *testing.T) {
	_, err := gitea.New(context.Background(), gitea.Options{Owner: "o", Repo: "r"})
	if !errors.Is(err, gitea.ErrMissingHost) {
		t.Errorf("err = %v, want ErrMissingHost", err)
	}
}
