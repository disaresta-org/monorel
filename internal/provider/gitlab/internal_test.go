package gitlab

import "testing"

func TestNormalizeHost(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"gitlab.com", "https://gitlab.com"},
		{"gitlab.example.com", "https://gitlab.example.com"},
		{"https://gitlab.com", "https://gitlab.com"},
		{"http://localhost:8080", "http://localhost:8080"},
		{"https://gitlab.com/", "https://gitlab.com"},
		{"gitlab.com/", "https://gitlab.com"},
		{"HTTPS://gitlab.com", "HTTPS://gitlab.com"},
		{"HTTP://localhost:8080", "HTTP://localhost:8080"},
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
