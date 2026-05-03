package cli

import (
	"strings"
	"testing"
)

func TestAuto_Help(t *testing.T) {
	stdout, _, err := runCmd(t, "auto", "--help")
	if err != nil {
		t.Fatalf("auto --help: %v", err)
	}
	for _, want := range []string{"auto", "release", "preview"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("help missing %q\n%s", want, stdout)
		}
	}
}

func TestAutoCmd_NoToken(t *testing.T) {
	// No provider token in env: auto returns a plain fmt.Errorf that
	// names the env var. (Unlike detect-release it does not wrap as
	// ErrExit; matches the existing preview --upsert behavior.)
	f := newFixture(t, singlePackageTOML, nil, nil)
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "")

	_, _, err := runCmd(t, "auto", "--config", f.configPath)
	if err == nil {
		t.Fatal("expected error for no-token")
	}
	if !strings.Contains(err.Error(), "GITHUB_TOKEN") {
		t.Errorf("err should mention GITHUB_TOKEN; got: %v", err)
	}
}
