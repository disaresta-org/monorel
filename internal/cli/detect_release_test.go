package cli

import (
	"errors"
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

func TestDetectRelease_NoToken(t *testing.T) {
	// No provider token in env: detect-release should exit 2 with a
	// stderr hint naming the env var. Exit 2 is the documented contract
	// for "detection / repo-state errors"; exit 1 is reserved for
	// "HEAD is not a release-PR merge".
	f := newFixture(t, singlePackageTOML, nil, nil)
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "")

	_, stderr, err := runCmd(t, "detect-release", "--config", f.configPath)
	if err == nil {
		t.Fatal("expected ErrExit(2) for no-token")
	}

	var ee ErrExit
	if !errors.As(err, &ee) || int(ee) != 2 {
		t.Errorf("err = %v, want ErrExit(2)", err)
	}

	if !strings.Contains(stderr, "GITHUB_TOKEN") {
		t.Errorf("stderr should mention GITHUB_TOKEN; got: %s", stderr)
	}
}
