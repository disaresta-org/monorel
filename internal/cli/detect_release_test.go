package cli

import (
	"strings"
	"testing"
)

func TestDetectRelease_Help(t *testing.T) {
	// Smoke test: the command registers and its help text mentions
	// the exit-code contract.
	stdout, _, err := runCmd(t, "detect-release", "--help")
	if err != nil {
		t.Fatalf("detect-release --help: %v", err)
	}
	for _, want := range []string{"detect-release", "Exit", "release"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("help missing %q\n%s", want, stdout)
		}
	}
}
