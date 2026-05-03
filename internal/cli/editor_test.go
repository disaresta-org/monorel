package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fakeEditor returns the path to a tiny shell script that, when
// invoked as `<script> <file>`, replaces the file's content with the
// given replacement bytes. Mirrors the contract a real editor honors:
// take a file argument, leave its final state on disk.
//
// On Windows we'd need a .bat shim; the editor tests skip there
// because the project's CI matrix runs on Linux only.
func fakeEditor(t *testing.T, replacement string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake-editor shim is sh-only; skipped on Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "fake-editor.sh")
	contents := "#!/bin/sh\nset -e\ncat > \"$1\" <<'__MONOREL_FAKE_EDITOR__'\n" +
		replacement + "\n__MONOREL_FAKE_EDITOR__\n"
	if err := os.WriteFile(path, []byte(contents), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestPromptViaEditor_HappyPath(t *testing.T) {
	editor := fakeEditor(t, "First line of the changelog body.\n\nSecond paragraph.")

	got, err := promptViaEditor("", editor)
	if err != nil {
		t.Fatalf("promptViaEditor: %v", err)
	}
	want := "First line of the changelog body.\n\nSecond paragraph."
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestPromptViaEditor_StripsCommentLines(t *testing.T) {
	// The editor wrote a body that includes commented prompt lines
	// (mirroring what a user who didn't touch the placeholder would
	// produce). They get stripped on read-back.
	editor := fakeEditor(t,
		"My change description.\n"+
			"# This is a placeholder line that should be dropped.\n"+
			"# Another placeholder.\n"+
			"More body content.")

	got, err := promptViaEditor("", editor)
	if err != nil {
		t.Fatalf("promptViaEditor: %v", err)
	}
	if strings.Contains(got, "#") {
		t.Errorf("comment-prefixed lines should be stripped; got: %q", got)
	}
	if !strings.Contains(got, "My change description.") {
		t.Errorf("body content missing; got: %q", got)
	}
	if !strings.Contains(got, "More body content.") {
		t.Errorf("body content missing; got: %q", got)
	}
}

func TestPromptViaEditor_TrimsSurroundingWhitespace(t *testing.T) {
	editor := fakeEditor(t, "\n\n   trimmed body   \n\n")

	got, err := promptViaEditor("", editor)
	if err != nil {
		t.Fatalf("promptViaEditor: %v", err)
	}
	if got != "trimmed body" {
		t.Errorf("got %q, want %q", got, "trimmed body")
	}
}

func TestPromptViaEditor_EmptyBodyOK(t *testing.T) {
	// User saved an empty file (or only commented lines). The
	// returned body is empty; the caller decides what to do with it.
	editor := fakeEditor(t, "# only comments here\n# nothing else")

	got, err := promptViaEditor("", editor)
	if err != nil {
		t.Fatalf("promptViaEditor: %v", err)
	}
	if got != "" {
		t.Errorf("expected empty body; got %q", got)
	}
}

func TestPromptViaEditor_EditorExitNonZeroIsAnError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh-only fake editor")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "fail.sh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 7\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := promptViaEditor("", path)
	if err == nil {
		t.Fatal("expected error when the editor exits non-zero")
	}
	if !strings.Contains(err.Error(), "editor:") {
		t.Errorf("error should be tagged with 'editor:'; got: %v", err)
	}
}

func TestResolveEditor_PrefersVisualOverEditor(t *testing.T) {
	t.Setenv("VISUAL", "my-visual-editor")
	t.Setenv("EDITOR", "my-fallback-editor")

	if got := resolveEditor(); got != "my-visual-editor" {
		t.Errorf("got %q, want VISUAL value", got)
	}
}

func TestResolveEditor_FallsBackToEditorWhenVisualUnset(t *testing.T) {
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "my-fallback-editor")

	if got := resolveEditor(); got != "my-fallback-editor" {
		t.Errorf("got %q, want EDITOR value", got)
	}
}

func TestResolveEditor_FallsBackToPlatformDefault(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Setenv("VISUAL", "")
		t.Setenv("EDITOR", "")
		if got := resolveEditor(); got != "notepad" {
			t.Errorf("got %q, want notepad on Windows", got)
		}
		return
	}
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "")
	got := resolveEditor()
	if got != "vi" && got != "nano" {
		t.Errorf("got %q, want vi or nano on Unix", got)
	}
}
