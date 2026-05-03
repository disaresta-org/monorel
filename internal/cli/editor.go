package cli

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// editorPlaceholder seeds the temp file the editor opens. Lines
// beginning with "#" are stripped on read-back, mirroring git
// commit's convention.
const editorPlaceholder = `

# Write the changelog body for this changeset.
# Markdown is supported, except that lines starting with "#" are
# stripped (so "# Heading" is dropped; use "## Heading" or higher).
# Save and close the editor to confirm; an empty body aborts.
`

// promptViaEditor opens $EDITOR (or $VISUAL, then a platform default)
// against a temp file pre-filled with initial content plus a commented
// prompt block. Returns the user's body with comment lines stripped
// and surrounding whitespace trimmed.
//
// editorOverride lets tests inject an editor command; production
// callers pass "".
func promptViaEditor(initial, editorOverride string) (string, error) {
	f, err := os.CreateTemp("", "monorel-changeset-*.md")
	if err != nil {
		return "", fmt.Errorf("editor: create temp file: %w", err)
	}
	tmpPath := f.Name()
	defer os.Remove(tmpPath)

	body := initial + editorPlaceholder
	if _, err := f.WriteString(body); err != nil {
		f.Close()
		return "", fmt.Errorf("editor: write temp file: %w", err)
	}
	if err := f.Close(); err != nil {
		return "", fmt.Errorf("editor: close temp file: %w", err)
	}

	editor := editorOverride
	if editor == "" {
		editor = resolveEditor()
	}
	if editor == "" {
		return "", errors.New("editor: no editor configured (set $VISUAL or $EDITOR)")
	}

	// The editor field can be a command line ("vim --noplugin"); split
	// on whitespace so we exec the binary with its flags.
	parts := strings.Fields(editor)
	cmd := exec.Command(parts[0], append(parts[1:], tmpPath)...) // #nosec G204 -- editor is operator-controlled
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("editor: %s: %w", editor, err)
	}

	out, err := readAndStripComments(tmpPath)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// resolveEditor returns the first non-empty match in priority order:
// $VISUAL, $EDITOR, then a platform fallback. Returns "" if no
// fallback is available.
//
// The returned value is exec'd by splitting on whitespace
// ([strings.Fields]); shell quoting is not honored. EDITOR values like
// `code --wait` work; values like `EDITOR='emacsclient -a "" -c'`
// (relying on quoted-empty-string args) need a wrapper script.
func resolveEditor() string {
	if v := os.Getenv("VISUAL"); v != "" {
		return v
	}
	if v := os.Getenv("EDITOR"); v != "" {
		return v
	}
	if runtime.GOOS == "windows" {
		return "notepad"
	}
	for _, candidate := range []string{"vi", "nano"} {
		if _, err := exec.LookPath(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

// readAndStripComments reads the file at path and returns its content
// with lines starting with "#" removed. Mirrors git commit's
// commented-line convention so users can leave the prompt block
// untouched and have it scrubbed automatically.
func readAndStripComments(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("editor: read back %s: %w", path, err)
	}
	defer f.Close()

	var b strings.Builder
	sc := bufio.NewScanner(f)
	// Default scanner buffer is 64KiB; raise to 1MiB so a paste of a
	// long changelog body doesn't truncate.
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "#") {
			continue
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	if err := sc.Err(); err != nil {
		return "", fmt.Errorf("editor: scan %s: %w", path, err)
	}
	return b.String(), nil
}
