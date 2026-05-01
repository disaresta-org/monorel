package cli

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	"monorel.disaresta.com/changeset"
)

func newPreCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pre",
		Short: "Manage pre-release mode (rc / beta / alpha).",
		Long: `Pre-release mode causes monorel release to append a "-<channel>.N" suffix
to next versions and increment N per release. While in pre-release mode,
no stable tag is produced; "monorel pre exit" returns to stable releases.

Subcommands:
  monorel pre enter <channel>   start pre-release mode (writes .changeset/pre.json)
  monorel pre exit              return to stable releases (clears state)
  monorel pre status            print current pre-release state, if any`,
	}

	cmd.AddCommand(newPreEnterCmd(), newPreExitCmd(), newPreStatusCmd())
	return cmd
}

func newPreEnterCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "enter <channel>",
		Short: "Start pre-release mode with the given channel name.",
		Long: `Writes .changeset/pre.json with the given channel ("rc", "beta",
"alpha", or any non-empty SemVer-2.0-compatible identifier).

Errors if pre-release mode is already active. To switch channels, run
"monorel pre exit" first.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			channel := args[0]
			rt, err := loadRuntime(cmd)
			if err != nil {
				return err
			}
			if rt.PreState != nil {
				return fmt.Errorf("already in pre-release mode (channel %q); run `monorel pre exit` first",
					rt.PreState.Channel)
			}
			state := &changeset.PreState{
				Mode:     "pre",
				Channel:  channel,
				Counters: map[string]int{},
			}
			if err := state.Write(rt.ChangesetDir); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(),
				"entered pre-release mode (channel %q). Subsequent releases will be tagged vX.Y.Z-%s.N.\n",
				channel, channel)
			return nil
		},
	}
}

func newPreExitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "exit",
		Short: "Exit pre-release mode. The next release will be a stable version.",
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := loadRuntime(cmd)
			if err != nil {
				return err
			}
			if rt.PreState == nil {
				fmt.Fprintln(cmd.OutOrStdout(), "not in pre-release mode; nothing to exit.")
				return nil
			}
			if err := changeset.RemovePreState(rt.ChangesetDir); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(),
				"exited pre-release mode (was channel %q). Next release is a stable version.\n",
				rt.PreState.Channel)
			return nil
		},
	}
}

func newPreStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Print the current pre-release mode state, if any.",
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := loadRuntime(cmd)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if rt.PreState == nil {
				fmt.Fprintln(out, "stable mode (not in pre-release).")
				return nil
			}
			fmt.Fprintf(out, "pre-release mode: channel=%q\n", rt.PreState.Channel)
			if len(rt.PreState.Counters) == 0 {
				fmt.Fprintln(out, "  no per-package counters yet.")
				return nil
			}
			// Sort counter keys for deterministic output.
			names := make([]string, 0, len(rt.PreState.Counters))
			for name := range rt.PreState.Counters {
				names = append(names, name)
			}
			sort.Strings(names)
			for _, name := range names {
				fmt.Fprintf(out, "  %s: counter=%d\n", name, rt.PreState.Counters[name])
			}
			return nil
		},
	}
}
