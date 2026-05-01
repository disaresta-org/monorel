package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/disaresta-org/monorel/internal/changeset"
	"github.com/disaresta-org/monorel/internal/config"
	"github.com/disaresta-org/monorel/internal/semver"
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
	fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", rel)
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

