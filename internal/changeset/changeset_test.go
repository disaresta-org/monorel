package changeset

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/disaresta-org/monorel/internal/semver"
)

func TestParse_Valid(t *testing.T) {
	input := `---
"pkg-a": minor
"pkg-b": patch
---

Add foo, fix bar.
`
	cs, err := Parse(strings.NewReader(input), "happy-cats")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cs.Name != "happy-cats" {
		t.Errorf("Name = %q, want happy-cats", cs.Name)
	}
	if got := cs.Bumps["pkg-a"]; got != semver.Minor {
		t.Errorf("pkg-a = %v, want Minor", got)
	}
	if got := cs.Bumps["pkg-b"]; got != semver.Patch {
		t.Errorf("pkg-b = %v, want Patch", got)
	}
	if cs.Body != "Add foo, fix bar." {
		t.Errorf("Body = %q, want %q", cs.Body, "Add foo, fix bar.")
	}
	want := []string{"pkg-a", "pkg-b"}
	got := cs.PackageNames()
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("PackageNames = %v, want %v", got, want)
	}
}

func TestParse_AllBumpLevels(t *testing.T) {
	input := `---
"a": major
"b": minor
"c": patch
---
body`
	cs, err := Parse(strings.NewReader(input), "x")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cs.Bumps["a"] != semver.Major {
		t.Error("a should be Major")
	}
	if cs.Bumps["b"] != semver.Minor {
		t.Error("b should be Minor")
	}
	if cs.Bumps["c"] != semver.Patch {
		t.Error("c should be Patch")
	}
}

func TestParse_CRLFLineEndings(t *testing.T) {
	input := "---\r\n\"pkg-a\": patch\r\n---\r\n\r\nbody\r\n"
	cs, err := Parse(strings.NewReader(input), "x")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cs.Bumps["pkg-a"] != semver.Patch {
		t.Errorf("pkg-a not parsed under CRLF")
	}
	if cs.Body != "body" {
		t.Errorf("Body = %q under CRLF", cs.Body)
	}
}

func TestParse_BOMTolerated(t *testing.T) {
	input := "\ufeff---\n\"a\": patch\n---\nbody\n"
	cs, err := Parse(strings.NewReader(input), "x")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cs.Bumps["a"] != semver.Patch {
		t.Errorf("a not parsed when BOM present")
	}
}

func TestParse_EmptyBodyOK(t *testing.T) {
	input := `---
"a": minor
---
`
	cs, err := Parse(strings.NewReader(input), "x")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cs.Body != "" {
		t.Errorf("Body = %q, want empty", cs.Body)
	}
}

func TestParse_Errors(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantSub string
	}{
		{
			name:    "missing frontmatter",
			input:   "no frontmatter here",
			wantSub: "frontmatter",
		},
		{
			name:    "missing closing fence",
			input:   "---\n\"a\": minor\n",
			wantSub: "frontmatter",
		},
		{
			name:    "empty bumps",
			input:   "---\n---\nbody",
			wantSub: "no packages",
		},
		{
			name:    "invalid bump level",
			input:   "---\n\"a\": breaking\n---\nbody",
			wantSub: "invalid bump level",
		},
		{
			name:    "malformed yaml",
			input:   "---\n: just colon\n---\nbody",
			wantSub: "yaml",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Parse(strings.NewReader(c.input), "x")
			if err == nil {
				t.Fatalf("expected error containing %q", c.wantSub)
			}
			if !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(c.wantSub)) {
				t.Errorf("err = %v\n want substring %q", err, c.wantSub)
			}
		})
	}
}

func TestWrite_RoundTrip(t *testing.T) {
	original := &Changeset{
		Name: "test-name",
		Bumps: map[string]semver.BumpLevel{
			"pkg-z": semver.Minor,
			"pkg-a": semver.Patch,
		},
		Body: "Body text.",
	}

	var buf bytes.Buffer
	if err := original.Write(&buf); err != nil {
		t.Fatalf("Write: %v", err)
	}

	roundTripped, err := Parse(&buf, "test-name")
	if err != nil {
		t.Fatalf("Parse round-trip: %v", err)
	}
	if roundTripped.Name != original.Name {
		t.Errorf("Name: got %q want %q", roundTripped.Name, original.Name)
	}
	if len(roundTripped.Bumps) != len(original.Bumps) {
		t.Errorf("Bumps len: got %d want %d", len(roundTripped.Bumps), len(original.Bumps))
	}
	for pkg, want := range original.Bumps {
		if got := roundTripped.Bumps[pkg]; got != want {
			t.Errorf("Bumps[%q]: got %v want %v", pkg, got, want)
		}
	}
	if roundTripped.Body != original.Body {
		t.Errorf("Body: got %q want %q", roundTripped.Body, original.Body)
	}
}

func TestWrite_DeterministicOrder(t *testing.T) {
	cs := &Changeset{
		Name: "x",
		Bumps: map[string]semver.BumpLevel{
			"zebra": semver.Patch,
			"apple": semver.Minor,
			"mango": semver.Major,
		},
		Body: "body",
	}

	var buf1, buf2 bytes.Buffer
	if err := cs.Write(&buf1); err != nil {
		t.Fatal(err)
	}
	if err := cs.Write(&buf2); err != nil {
		t.Fatal(err)
	}
	if buf1.String() != buf2.String() {
		t.Errorf("non-deterministic Write output:\n%s\n---\n%s", buf1.String(), buf2.String())
	}
	// First Bumps line should be apple (alphabetical).
	idx := strings.Index(buf1.String(), "\n")
	firstLine := buf1.String()[idx+1:]
	if !strings.HasPrefix(firstLine, "\"apple\":") {
		t.Errorf("first Bumps entry should be apple, got line: %q", firstLine[:30])
	}
}

func TestWrite_EmptyBumpsErrors(t *testing.T) {
	cs := &Changeset{Name: "x", Bumps: nil, Body: "body"}
	var buf bytes.Buffer
	if err := cs.Write(&buf); err == nil {
		t.Errorf("expected error for empty bumps")
	}
}

func TestLoadAll(t *testing.T) {
	dir := t.TempDir()

	// One real changeset.
	cs := &Changeset{
		Name:  "happy-cats",
		Bumps: map[string]semver.BumpLevel{"pkg-a": semver.Minor},
		Body:  "feature",
	}
	if err := cs.WriteFile(dir); err != nil {
		t.Fatal(err)
	}
	// Reserved file: ignored.
	if err := os.WriteFile(filepath.Join(dir, "pre.json"), []byte(`{"mode":"pre","channel":"rc"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	// Non-md file: ignored.
	if err := os.WriteFile(filepath.Join(dir, "scratch.txt"), []byte("scratch"), 0o644); err != nil {
		t.Fatal(err)
	}
	// .gitkeep: ignored.
	if err := os.WriteFile(filepath.Join(dir, ".gitkeep"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := LoadAll(dir)
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].Name != "happy-cats" {
		t.Errorf("Name = %q, want happy-cats", got[0].Name)
	}
	if got[0].Bumps["pkg-a"] != semver.Minor {
		t.Error("expected pkg-a Minor bump")
	}
}

func TestLoadAll_MissingDirIsEmpty(t *testing.T) {
	cs, err := LoadAll(filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil {
		t.Fatalf("LoadAll on missing dir: %v", err)
	}
	if len(cs) != 0 {
		t.Errorf("got %d changesets, want 0", len(cs))
	}
}

func TestLoadAll_SortedByName(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"zebra", "apple", "mango"} {
		cs := &Changeset{Name: n, Bumps: map[string]semver.BumpLevel{"x": semver.Patch}, Body: "b"}
		if err := cs.WriteFile(dir); err != nil {
			t.Fatal(err)
		}
	}
	got, err := LoadAll(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"apple", "mango", "zebra"}
	for i := range got {
		if got[i].Name != want[i] {
			t.Errorf("[%d] = %q, want %q", i, got[i].Name, want[i])
		}
	}
}

func TestRandomName(t *testing.T) {
	for i := 0; i < 50; i++ {
		name := RandomName()
		if !IsValidName(name) {
			t.Errorf("RandomName produced invalid name: %q", name)
		}
		if !strings.Contains(name, "-") {
			t.Errorf("RandomName missing hyphen: %q", name)
		}
	}
}

func TestIsValidName(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"happy-cats", true},
		{"swift-pandas", true},
		{"a", true},
		{"abc-123", true},
		{"", false},
		{"foo/bar", false},
		{"foo..bar", false},
		{"-leading", false},
		{"trailing-", false},
		{"double--hyphen", false},
		{"UPPER", false},
		{"with space", false},
		{"with.dot", false},
	}
	for _, c := range cases {
		if got := IsValidName(c.in); got != c.want {
			t.Errorf("IsValidName(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestPreState_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	original := &PreState{
		Mode:    "pre",
		Channel: "rc",
		Counters: map[string]int{
			"pkg-a": 0,
			"pkg-b": 3,
		},
	}
	if err := original.Write(dir); err != nil {
		t.Fatal(err)
	}

	got, err := LoadPreState(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Mode != "pre" || got.Channel != "rc" {
		t.Errorf("got %+v", got)
	}
	if got.Counters["pkg-a"] != 0 || got.Counters["pkg-b"] != 3 {
		t.Errorf("counters: %+v", got.Counters)
	}
}

func TestPreState_MissingIsNil(t *testing.T) {
	state, err := LoadPreState(t.TempDir())
	if err != nil {
		t.Fatalf("LoadPreState on empty dir: %v", err)
	}
	if state != nil {
		t.Errorf("expected nil for empty dir, got %+v", state)
	}
}

func TestPreState_RejectsInvalid(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name    string
		content string
		wantSub string
	}{
		{
			name:    "wrong mode",
			content: `{"mode":"normal","channel":"rc"}`,
			wantSub: "unknown mode",
		},
		{
			name:    "empty channel",
			content: `{"mode":"pre","channel":""}`,
			wantSub: "channel is empty",
		},
		{
			name:    "malformed json",
			content: `{not json}`,
			wantSub: "parse",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(dir, PreStateFilename)
			if err := os.WriteFile(path, []byte(c.content), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := LoadPreState(dir)
			if err == nil {
				t.Fatalf("expected error containing %q", c.wantSub)
			}
			if !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(c.wantSub)) {
				t.Errorf("err = %v\n want substring %q", err, c.wantSub)
			}
		})
	}
}

func TestPreState_RemoveIdempotent(t *testing.T) {
	dir := t.TempDir()
	if err := RemovePreState(dir); err != nil {
		t.Errorf("Remove on empty dir: %v", err)
	}
	state := &PreState{Mode: "pre", Channel: "rc"}
	if err := state.Write(dir); err != nil {
		t.Fatal(err)
	}
	if err := RemovePreState(dir); err != nil {
		t.Errorf("Remove on existing: %v", err)
	}
	got, _ := LoadPreState(dir)
	if got != nil {
		t.Errorf("Remove did not delete: %+v", got)
	}
}

func TestPreState_IncrementCounter(t *testing.T) {
	state := &PreState{Mode: "pre", Channel: "rc"}
	first := state.IncrementCounter("pkg-a")
	if first != 0 {
		t.Errorf("first IncrementCounter = %d, want 0", first)
	}
	second := state.IncrementCounter("pkg-a")
	if second != 1 {
		t.Errorf("second = %d, want 1", second)
	}
	third := state.IncrementCounter("pkg-a")
	if third != 2 {
		t.Errorf("third = %d, want 2", third)
	}
	other := state.IncrementCounter("pkg-b")
	if other != 0 {
		t.Errorf("pkg-b first = %d, want 0", other)
	}
	if state.Counters["pkg-a"] != 3 {
		t.Errorf("pkg-a counter after 3 increments = %d, want 3", state.Counters["pkg-a"])
	}
}

func TestPreState_CounterForNilOrMissing(t *testing.T) {
	var nilState *PreState
	if got := nilState.CounterFor("anything"); got != 0 {
		t.Errorf("CounterFor on nil = %d, want 0", got)
	}
	state := &PreState{Mode: "pre", Channel: "rc"}
	if got := state.CounterFor("missing"); got != 0 {
		t.Errorf("CounterFor on missing = %d, want 0", got)
	}
}
