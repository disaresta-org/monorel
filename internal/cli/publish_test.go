package cli

import (
	"strings"
	"testing"
)

func TestPublishCmd_NoTagsAtHead(t *testing.T) {
	f := newFixture(t, singlePackageTOML, nil, nil)
	stdout, _, err := runCmd(t, "publish", "--config", f.configPath)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if !strings.Contains(stdout, "No tags at HEAD") {
		t.Errorf("empty case should be a clean noop: %s", stdout)
	}
}

func TestPublishCmd_TagsAtHead_NoToken(t *testing.T) {
	f := newFixture(t,
		singlePackageTOML,
		nil,
		// Tag pointing at HEAD will be picked up by DiscoverPublishables.
		[]string{"transports/foo/v1.0.0"},
	)
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "")

	_, _, err := runCmd(t, "publish", "--config", f.configPath)
	if err == nil {
		t.Fatal("expected error when no token is set")
	}
	if !strings.Contains(err.Error(), "GITHUB_TOKEN or GH_TOKEN") {
		t.Errorf("error %q should name env vars", err)
	}
}
