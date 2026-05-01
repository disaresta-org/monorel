package gitea

import "testing"

// In-package test file (package gitea, not gitea_test) so
// unexported helpers like normalizeHost are reachable. The black-box
// tests live in gitea_test.go.

func TestNormalizeHost(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		// Bare hostname defaults to https://.
		{"gitea.example.com", "https://gitea.example.com"},
		{"localhost:3000", "https://localhost:3000"},
		// Fully-qualified URLs are passed through.
		{"https://gitea.example.com", "https://gitea.example.com"},
		{"http://localhost:3000", "http://localhost:3000"},
		// Trailing slashes get stripped both forms.
		{"https://gitea.example.com/", "https://gitea.example.com"},
		{"gitea.example.com/", "https://gitea.example.com"},
		// Sub-path-mounted Gitea (behind a reverse proxy).
		{"http://localhost:3000/gitea", "http://localhost:3000/gitea"},
		// Uppercase scheme. Treated as scheme'd, not bare.
		{"HTTPS://gitea.example.com", "HTTPS://gitea.example.com"},
		{"HTTP://localhost:3000", "HTTP://localhost:3000"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got := normalizeHost(tc.in)
			if got != tc.want {
				t.Errorf("normalizeHost(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
