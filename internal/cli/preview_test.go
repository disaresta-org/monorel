package cli

import (
	"strings"
	"testing"
)

func TestPreviewCmd_StdoutOnly_Empty(t *testing.T) {
	f := newFixture(t, singlePackageTOML, nil, []string{"transports/foo/v1.0.0"})
	stdout, _, err := runCmd(t, "preview", "--config", f.configPath)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if !strings.Contains(stdout, "No pending changesets") {
		t.Errorf("empty plan should render closing-PR copy: %s", stdout)
	}
}

func TestPreviewCmd_StdoutOnly_NonEmpty(t *testing.T) {
	f := newFixture(t,
		singlePackageTOML,
		map[string]string{
			"first": "---\n\"foo\": minor\n---\n\nAdds the foo flag.\n",
		},
		[]string{"transports/foo/v1.5.0"},
	)
	stdout, _, err := runCmd(t, "preview", "--config", f.configPath)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	for _, want := range []string{
		"opened by [monorel]",
		"`foo`",
		"v1.5.0",
		"**v1.6.0**",
		"### foo v1.6.0",
		"Adds the foo flag.",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("preview body missing %q\nfull:\n%s", want, stdout)
		}
	}
}

func TestPreviewCmd_Upsert_NoToken(t *testing.T) {
	f := newFixture(t,
		singlePackageTOML,
		map[string]string{"first": "---\n\"foo\": patch\n---\n\nFix.\n"},
		nil,
	)
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "")

	_, _, err := runCmd(t, "preview", "--config", f.configPath, "--upsert")
	if err == nil {
		t.Fatal("expected error when no token is set")
	}
	if !strings.Contains(err.Error(), "GITHUB_TOKEN or GH_TOKEN") {
		t.Errorf("error %q should name the env vars in human-readable form", err)
	}
}
