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
