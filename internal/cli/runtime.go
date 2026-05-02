package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"go.loglayer.dev/plugins/fmtlog/v2"
	"go.loglayer.dev/transports/cli/v2"
	loglayer "go.loglayer.dev/v2"

	"monorel.disaresta.com/changeset"
	"monorel.disaresta.com/config"
	"monorel.disaresta.com/internal/git"
)

// Runtime is the shared state that read-only commands (plan, status,
// preview) compute by combining monorel.toml + .changeset/*.md + git
// tags. Constructed via [loadRuntime].
type Runtime struct {
	// ConfigPath is the absolute path to monorel.toml.
	ConfigPath string

	// RepoDir is the directory that contains monorel.toml. Treated
	// as the repository root for git invocations and the parent of
	// .changeset/.
	RepoDir string

	// Config is the parsed monorel.toml.
	Config *config.Config

	// Repo is a git.Repo bound to RepoDir.
	Repo git.Repo

	// Changesets are pending changesets loaded from .changeset/.
	Changesets []*changeset.Changeset

	// Tags is every tag in the repository.
	Tags []string

	// PreState is the pre-release-mode state loaded from
	// .changeset/pre.json, or nil when not in pre-release mode.
	// Read-only; pre subcommands construct their own to write.
	PreState *changeset.PreState

	// ChangesetDir is the absolute path to .changeset/. The pre and
	// release commands write/delete files here.
	ChangesetDir string

	// Log is the CLI logger. Use this for all user-facing output.
	// Info / debug routes to the cobra command's stdout; warn /
	// error / fatal routes to its stderr.
	Log *loglayer.LogLayer
}

// configPathFlagName is the persistent --config flag value. Set on
// the root command in [newRootCmd] via the persistent flag set.
const configPathFlagName = "config"

// Logger-related flag names. Persistent across every subcommand.
const (
	colorFlagName   = "color"
	verboseFlagName = "verbose"
	quietFlagName   = "quiet"
)

// addPersistentFlags registers flags shared across every subcommand.
// Called from newRootCmd.
func addPersistentFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().String(configPathFlagName, "monorel.toml",
		"Path to monorel.toml (the repository root is its parent directory).")
	cmd.PersistentFlags().String(colorFlagName, "auto",
		"Colorize CLI output: auto (TTY-detected), always, or never.")
	cmd.PersistentFlags().CountP(verboseFlagName, "v",
		"Increase verbosity: -v adds debug, -vv also appends structured fields. Default shows info, warn, error.")
	cmd.PersistentFlags().BoolP(quietFlagName, "q", false,
		"Quiet mode: suppress info and warn output, leaving only error / fatal.")
}

// newLogger constructs a configured CLI logger from the cobra command
// and the global flags. stdout / stderr default to cmd.OutOrStdout()
// and cmd.ErrOrStderr() so cobra's test harness can capture output.
func newLogger(cmd *cobra.Command) (*loglayer.LogLayer, error) {
	colorFlag, err := cmd.Flags().GetString(colorFlagName)
	if err != nil {
		return nil, err
	}
	verbosity, err := cmd.Flags().GetCount(verboseFlagName)
	if err != nil {
		return nil, err
	}
	quiet, err := cmd.Flags().GetBool(quietFlagName)
	if err != nil {
		return nil, err
	}

	color, err := parseColorMode(colorFlag)
	if err != nil {
		return nil, err
	}

	t := cli.New(cli.Config{
		Stdout:     cmd.OutOrStdout(),
		Stderr:     cmd.ErrOrStderr(),
		Color:      color,
		ShowFields: verbosity >= 2,
	})

	log := loglayer.New(loglayer.Config{
		Transport: t,
		Plugins:   []loglayer.Plugin{fmtlog.New()},
	})

	// -q wins over -v: a script that pipes monorel into something
	// else still gets `monorel -v -q ...` cleanly silenced when
	// callers compose flags. This matches `kubectl`, `gh`, and
	// `cargo`.
	switch {
	case quiet:
		log.SetLevel(loglayer.LogLevelError)
	case verbosity >= 1:
		log.SetLevel(loglayer.LogLevelDebug)
	default:
		log.SetLevel(loglayer.LogLevelInfo)
	}
	return log, nil
}

// parseColorMode maps the --color flag value to cli.ColorMode.
// Accepts "auto", "always", and "never" (case-insensitive). Empty
// string defaults to auto so a missing flag value is forgiving.
func parseColorMode(s string) (cli.ColorMode, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "auto":
		return cli.ColorAuto, nil
	case "always":
		return cli.ColorAlways, nil
	case "never":
		return cli.ColorNever, nil
	default:
		return cli.ColorAuto, fmt.Errorf("--color: %q is not one of auto|always|never", s)
	}
}

// loadRuntime resolves the --config flag, loads everything a read-only
// command needs, and returns it bundled. Does NOT validate that the
// working tree is clean — release-side commands do that themselves.
func loadRuntime(cmd *cobra.Command) (*Runtime, error) {
	log, err := newLogger(cmd)
	if err != nil {
		return nil, err
	}

	configPath, err := cmd.Flags().GetString(configPathFlagName)
	if err != nil {
		return nil, err
	}
	configPath, err = filepath.Abs(configPath)
	if err != nil {
		return nil, fmt.Errorf("resolve --config: %w", err)
	}
	if _, err := os.Stat(configPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("monorel.toml not found at %s; run `monorel init` first", configPath)
		}
		return nil, fmt.Errorf("stat %s: %w", configPath, err)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, err
	}

	repoDir := filepath.Dir(configPath)
	repo := git.Open(repoDir)

	changesetDir := filepath.Join(repoDir, ".changeset")
	changesets, err := changeset.LoadAll(changesetDir)
	if err != nil {
		return nil, fmt.Errorf("load changesets: %w", err)
	}

	preState, err := changeset.LoadPreState(changesetDir)
	if err != nil {
		return nil, fmt.Errorf("load pre-release state: %w", err)
	}

	tags, err := repo.ListTags("")
	if err != nil {
		return nil, fmt.Errorf("list tags: %w", err)
	}

	return &Runtime{
		ConfigPath:   configPath,
		RepoDir:      repoDir,
		Config:       cfg,
		Repo:         repo,
		Changesets:   changesets,
		Tags:         tags,
		PreState:     preState,
		ChangesetDir: changesetDir,
		Log:          log,
	}, nil
}
