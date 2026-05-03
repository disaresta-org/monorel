package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"monorel.disaresta.com/changeset"
	"monorel.disaresta.com/config"
	"monorel.disaresta.com/semver"
)

func newAddCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Create a new changeset describing pending package changes.",
		Long: `Writes a .changeset/<name>.md file declaring which packages this PR
affects and at what bump level (major/minor/patch).

Non-interactive (CI / scripted):
    monorel add --package foo:minor --package bar:patch --message "Fix it."

Interactive (human author): omit --package and answer the prompts.`,
		RunE: runAdd,
	}
	cmd.Flags().StringArrayP("package", "p", nil,
		"Package and bump level, formatted name:level. Repeatable. "+
			"Triggers non-interactive mode when given.")
	cmd.Flags().StringP("message", "m", "",
		"Changeset body (the changelog entry text). May be empty.")
	cmd.Flags().String("name", "",
		"Override the auto-generated changeset filename (without .md).")
	return cmd
}

func runAdd(cmd *cobra.Command, _ []string) error {
	rt, err := loadRuntime(cmd)
	if err != nil {
		return err
	}

	pkgFlags, _ := cmd.Flags().GetStringArray("package")
	message, _ := cmd.Flags().GetString("message")
	name, _ := cmd.Flags().GetString("name")

	var bumps map[string]semver.BumpLevel
	if len(pkgFlags) > 0 {
		bumps, err = parsePackageFlags(pkgFlags, rt.Config)
		if err != nil {
			return err
		}
	} else if isReaderTTY(cmd.InOrStdin()) && isWriterTTY(cmd.ErrOrStderr()) {
		// Real human at a terminal: drive a huh form (multi-select +
		// per-package bump select + multi-line text). Falls through
		// to the bufio path on piped/redirected stdio so scripted use
		// (echo "1\nminor\n..." | monorel add) keeps working.
		//
		// Gate is on stdin + stderr (not stdout) because huh's
		// underlying tea program writes the form UI to stderr by
		// default; checking stdout would falsely allow `monorel add
		// 2>/tmp/log` (form invisible).
		bumps, message, err = promptHuh(cmd.InOrStdin(), cmd.ErrOrStderr(), rt.Config)
		if err != nil {
			return err
		}
	} else {
		bumps, message, err = promptInteractive(cmd.InOrStdin(), cmd.OutOrStdout(), rt.Config)
		if err != nil {
			return err
		}
	}

	if len(bumps) == 0 {
		return errors.New("no packages selected")
	}
	if name == "" {
		name = changeset.RandomName()
	} else if !changeset.IsValidName(name) {
		return fmt.Errorf("invalid --name %q (use lowercase letters, digits, hyphens)", name)
	}

	cs := &changeset.Changeset{Name: name, Bumps: bumps, Body: message}
	dir := filepath.Join(rt.RepoDir, ".changeset")
	if err := cs.WriteFile(dir); err != nil {
		return err
	}

	rel, err := filepath.Rel(rt.RepoDir, filepath.Join(dir, name+".md"))
	if err != nil {
		rel = name + ".md"
	}
	rt.Log.Info("wrote %s", rel)
	return nil
}

// parsePackageFlags converts each "name:level" string into a map
// entry. Returns an error on bad shape, unknown package, unknown bump
// level, or duplicate package across flags.
func parsePackageFlags(flags []string, cfg *config.Config) (map[string]semver.BumpLevel, error) {
	out := make(map[string]semver.BumpLevel, len(flags))
	for _, raw := range flags {
		name, levelStr, ok := strings.Cut(raw, ":")
		if !ok {
			return nil, fmt.Errorf("--package %q: expected name:level", raw)
		}
		name = strings.TrimSpace(name)
		levelStr = strings.TrimSpace(levelStr)
		if _, exists := cfg.Packages[name]; !exists {
			return nil, fmt.Errorf("--package %q: unknown package %q", raw, name)
		}
		if _, dup := out[name]; dup {
			return nil, fmt.Errorf("--package %q: package %q already specified", raw, name)
		}
		level, err := semver.ParseBumpLevel(levelStr)
		if err != nil {
			return nil, fmt.Errorf("--package %q: %w", raw, err)
		}
		out[name] = level
	}
	return out, nil
}

// promptInteractive runs a text-based prompt flow over (in, out). Picks
// packages from a numbered list, then a bump level for each, then a
// multi-line body terminated by a blank line.
func promptInteractive(in io.Reader, out io.Writer, cfg *config.Config) (map[string]semver.BumpLevel, string, error) {
	names := cfg.PackageNames()
	if len(names) == 0 {
		return nil, "", errors.New("monorel.toml declares no packages")
	}

	rd := bufio.NewReader(in)
	fmt.Fprintln(out, "Available packages:")
	for i, name := range names {
		fmt.Fprintf(out, "  %d) %s\n", i+1, name)
	}
	fmt.Fprint(out, "Pick packages (space- or comma-separated numbers): ")
	picksLine, err := rd.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, "", err
	}
	picked, err := parsePicks(picksLine, names)
	if err != nil {
		return nil, "", err
	}

	bumps := make(map[string]semver.BumpLevel, len(picked))
	for _, name := range picked {
		level, err := promptBump(rd, out, name)
		if err != nil {
			return nil, "", err
		}
		bumps[name] = level
	}

	fmt.Fprintln(out, "Body (end with a single empty line):")
	body, err := readMultiline(rd)
	if err != nil {
		return nil, "", err
	}
	return bumps, body, nil
}

// parsePicks turns "1, 3 4" into the corresponding sorted unique names
// from the supplied package list. 1-indexed; out-of-range and
// non-numeric tokens error.
func parsePicks(line string, names []string) ([]string, error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil, errors.New("no packages selected")
	}
	// Allow either commas or spaces (or both) as separators.
	tokens := strings.FieldsFunc(line, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' })
	seen := make(map[string]bool)
	var picked []string
	for _, t := range tokens {
		n, err := strconv.Atoi(t)
		if err != nil {
			return nil, fmt.Errorf("invalid pick %q: not a number", t)
		}
		if n < 1 || n > len(names) {
			return nil, fmt.Errorf("invalid pick %d: out of range [1, %d]", n, len(names))
		}
		name := names[n-1]
		if !seen[name] {
			seen[name] = true
			picked = append(picked, name)
		}
	}
	sort.Strings(picked)
	return picked, nil
}

// promptBump prompts the user for a bump level for the named package
// and parses the result. Single attempt; the caller's outer loop is
// the user re-running `monorel add` if they typo.
func promptBump(rd *bufio.Reader, out io.Writer, name string) (semver.BumpLevel, error) {
	fmt.Fprintf(out, "Bump level for %s (major|minor|patch): ", name)
	line, err := rd.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return semver.None, err
	}
	level, err := semver.ParseBumpLevel(strings.TrimSpace(line))
	if err != nil {
		return semver.None, fmt.Errorf("%s: %w", name, err)
	}
	return level, nil
}

// isReaderTTY reports whether r is an *os.File on an interactive
// terminal. Anything that isn't an *os.File (strings.Reader,
// bytes.Buffer, pipe wrappers) is non-TTY by definition.
func isReaderTTY(r io.Reader) bool {
	f, ok := r.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}

// isWriterTTY mirrors isReaderTTY for io.Writer.
func isWriterTTY(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}

// promptHuh runs the huh-driven form for `monorel add`: a multi-select
// over package names, then a Select per chosen package for the bump
// level, then a Text field for the changelog body. Each form is run
// independently so the bump-level questions can be generated from the
// multi-select's result.
//
// in/out are wired into each form via WithInput/WithOutput so callers
// (tests, embedded use cases) can drive the form deterministically
// without monkey-patching os.Stdin/os.Stderr. Production wires
// cmd.InOrStdin() / cmd.ErrOrStderr().
//
// huh.ErrUserAborted (Ctrl-C) is mapped to ErrExit(130) so main()
// returns a clean SIGINT exit code without printing "Error: user
// aborted".
func promptHuh(in io.Reader, out io.Writer, cfg *config.Config) (map[string]semver.BumpLevel, string, error) {
	names := cfg.PackageNames()
	if len(names) == 0 {
		return nil, "", errors.New("monorel.toml declares no packages")
	}

	options := make([]huh.Option[string], 0, len(names))
	for _, n := range names {
		options = append(options, huh.NewOption(n, n))
	}

	wire := func(f *huh.Form) *huh.Form { return f.WithInput(in).WithOutput(out) }

	var picked []string
	pickForm := wire(huh.NewForm(huh.NewGroup(
		huh.NewMultiSelect[string]().
			Title("Which packages does this changeset affect?").
			Description("Space toggles, enter confirms.").
			Options(options...).
			Validate(func(s []string) error {
				if len(s) == 0 {
					return errors.New("pick at least one package")
				}
				return nil
			}).
			Value(&picked),
	)))
	if err := runHuhForm(pickForm); err != nil {
		return nil, "", err
	}

	bumps := make(map[string]semver.BumpLevel, len(picked))
	for _, name := range picked {
		var level string
		bumpForm := wire(huh.NewForm(huh.NewGroup(
			huh.NewSelect[string]().
				Title(fmt.Sprintf("Bump level for %s", name)).
				Options(
					// Patch is the explicit default (Selected): if a
					// user hits enter without scrolling, the safe
					// least-bump applies, matching the principle of
					// least surprise.
					huh.NewOption("patch (bug fix, no API change)", "patch").Selected(true),
					huh.NewOption("minor (new feature, backward-compatible)", "minor"),
					huh.NewOption("major (breaking change)", "major"),
				).
				Value(&level),
		)))
		if err := runHuhForm(bumpForm); err != nil {
			return nil, "", err
		}
		l, err := semver.ParseBumpLevel(level)
		if err != nil {
			return nil, "", fmt.Errorf("%s: %w", name, err)
		}
		bumps[name] = l
	}

	var body string
	bodyForm := wire(huh.NewForm(huh.NewGroup(
		huh.NewText().
			Title("Changelog body").
			Description("Markdown. This becomes the entry under the bump-level heading.").
			CharLimit(0).
			Value(&body),
	)))
	if err := runHuhForm(bodyForm); err != nil {
		return nil, "", err
	}
	return bumps, strings.TrimSpace(body), nil
}

// runHuhForm runs a huh.Form, translating huh.ErrUserAborted into
// ErrExit(130) so Ctrl-C produces a clean SIGINT exit without the
// stderr "Error: user aborted" line.
func runHuhForm(f *huh.Form) error {
	if err := f.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return ErrExit(130)
		}
		return err
	}
	return nil
}

// readMultiline reads lines from rd until a single blank line, then
// returns the joined body trimmed of leading/trailing whitespace.
// EOF terminates the read just like a blank line.
func readMultiline(rd *bufio.Reader) (string, error) {
	var b strings.Builder
	for {
		line, err := rd.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return "", err
		}
		trimmed := strings.TrimRight(line, "\r\n")
		if trimmed == "" {
			break
		}
		b.WriteString(trimmed)
		b.WriteByte('\n')
		if errors.Is(err, io.EOF) {
			break
		}
	}
	return strings.TrimSpace(b.String()), nil
}
