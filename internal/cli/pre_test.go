package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"monorel.disaresta.com/internal/changeset"
)

func readPreState(t *testing.T, repoDir string) *changeset.PreState {
	t.Helper()
	path := filepath.Join(repoDir, ".changeset", "pre.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var s changeset.PreState
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatalf("parse pre.json: %v", err)
	}
	return &s
}

func TestPreCmd_EnterExitStatus(t *testing.T) {
	f := newFixture(t, twoPackageTOML, nil, nil)

	stdout, _, err := runCmd(t, "pre", "status", "--config", f.configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, "stable mode") {
		t.Errorf("status before enter should report stable mode; got: %s", stdout)
	}

	stdout, _, err = runCmd(t, "pre", "enter", "rc", "--config", f.configPath)
	if err != nil {
		t.Fatalf("enter: %v", err)
	}
	if !strings.Contains(stdout, "entered pre-release mode") {
		t.Errorf("enter output: %s", stdout)
	}
	state := readPreState(t, f.r.Dir)
	if state == nil {
		t.Fatal("pre.json not written")
	}
	if state.Channel != "rc" || state.Mode != "pre" {
		t.Errorf("state = %+v, want mode=pre channel=rc", state)
	}

	// Status now reports the channel.
	stdout, _, err = runCmd(t, "pre", "status", "--config", f.configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, "channel=\"rc\"") {
		t.Errorf("status output: %s", stdout)
	}

	// Re-entering errors.
	_, _, err = runCmd(t, "pre", "enter", "beta", "--config", f.configPath)
	if err == nil {
		t.Fatal("expected error on second enter")
	}
	if !strings.Contains(err.Error(), "already in pre-release mode") {
		t.Errorf("error = %q, want one mentioning already-in-pre", err)
	}

	// Exit succeeds and removes the file.
	stdout, _, err = runCmd(t, "pre", "exit", "--config", f.configPath)
	if err != nil {
		t.Fatalf("exit: %v", err)
	}
	if !strings.Contains(stdout, "exited pre-release mode") {
		t.Errorf("exit output: %s", stdout)
	}
	if _, err := os.Stat(filepath.Join(f.r.Dir, ".changeset", "pre.json")); !os.IsNotExist(err) {
		t.Errorf("pre.json still exists after exit: %v", err)
	}

	// Exit again is a no-op.
	stdout, _, err = runCmd(t, "pre", "exit", "--config", f.configPath)
	if err != nil {
		t.Fatalf("exit (idempotent): %v", err)
	}
	if !strings.Contains(stdout, "not in pre-release mode") {
		t.Errorf("idempotent exit output: %s", stdout)
	}
}

func TestPreCmd_PlanReflectsPreState(t *testing.T) {
	f := newFixture(t,
		singlePackageTOML,
		map[string]string{
			"first": "---\n\"foo\": minor\n---\n\nFeature.\n",
		},
		[]string{"transports/foo/v1.5.0"},
	)

	if _, _, err := runCmd(t, "pre", "enter", "rc", "--config", f.configPath); err != nil {
		t.Fatal(err)
	}

	stdout, _, err := runCmd(t, "plan", "--config", f.configPath, "--json")
	if err != nil {
		t.Fatal(err)
	}
	var got jsonPlan
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("unmarshal: %v\noutput:\n%s", err, stdout)
	}
	if len(got.Releases) != 1 {
		t.Fatalf("Releases len = %d, want 1", len(got.Releases))
	}
	if got.Releases[0].To != "v1.6.0-rc.0" {
		t.Errorf("To = %q, want v1.6.0-rc.0", got.Releases[0].To)
	}
	if got.Releases[0].Tag != "transports/foo/v1.6.0-rc.0" {
		t.Errorf("Tag = %q, want transports/foo/v1.6.0-rc.0", got.Releases[0].Tag)
	}
}
