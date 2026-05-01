package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/spf13/cobra"

	"monorel.disaresta.com/internal/git"
	"monorel.disaresta.com/validate"
)

func newValidateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Run static checks against monorel.toml + .changeset/*.md and report findings.",
		Long: `Walks the configured packages, the changeset directory, and (optionally)
the local tag namespace, surfacing every issue in one pass:

  - Schema: forge fields, package fields, no duplicate tag prefixes.
  - Filesystem: every package's path exists, no two packages share a
    path, every changelog's parent directory exists.
  - Changesets: every .changeset/*.md parses cleanly and only names
    packages declared in monorel.toml.
  - Tags (opt-in via --check-tags): every tag matching a package's
    prefix parses as valid semver. Non-semver tags surface as warnings.

Exit codes:
  0  no findings (or warnings without --strict).
  1  one or more errors.
  2  warnings only AND --strict was passed.

Pass --json for machine-readable output suitable for pre-commit hooks
and CI integrations. The JSON schema is the public Finding type's
encoding; new fields are additive across versions.`,
		RunE: runValidate,
	}
	cmd.Flags().Bool("json", false, "Emit findings as JSON.")
	cmd.Flags().Bool("strict", false, "Treat warnings as failures (exit 2 instead of 0).")
	cmd.Flags().Bool("check-tags", false,
		"Also validate the local tag namespace against each package's prefix. Requires a git repo at the config's parent directory.")
	return cmd
}

func runValidate(cmd *cobra.Command, _ []string) error {
	configPath, err := cmd.Flags().GetString(configPathFlagName)
	if err != nil {
		return err
	}
	asJSON, _ := cmd.Flags().GetBool("json")
	strict, _ := cmd.Flags().GetBool("strict")
	checkTags, _ := cmd.Flags().GetBool("check-tags")

	in := validate.Inputs{
		ConfigPath: configPath,
		CheckTags:  checkTags,
	}
	if checkTags {
		abs, err := filepath.Abs(configPath)
		if err != nil {
			return fmt.Errorf("resolve --config: %w", err)
		}
		repo := git.Open(filepath.Dir(abs))
		in.ListTags = func(prefix string) ([]string, error) { return repo.ListTags(prefix) }
	}

	findings := validate.Run(in)

	if asJSON {
		if err := writeFindingsJSON(cmd.OutOrStdout(), findings); err != nil {
			return err
		}
	} else {
		if err := writeFindingsText(cmd.OutOrStdout(), findings); err != nil {
			return err
		}
	}

	if validate.HasErrors(findings) {
		return ErrExit(1)
	}
	if strict && validate.HasWarnings(findings) {
		return ErrExit(2)
	}
	return nil
}

// writeFindingsText writes a human-readable summary. Empty findings
// produce "No findings." Warnings and errors are grouped by severity
// in fixed order (errors first, warnings second).
func writeFindingsText(w io.Writer, findings []validate.Finding) error {
	if len(findings) == 0 {
		_, err := fmt.Fprintln(w, "No findings. monorel.toml + .changeset/*.md look valid.")
		return err
	}

	errCount := 0
	warnCount := 0
	for _, f := range findings {
		switch f.Severity {
		case validate.SeverityError:
			errCount++
		case validate.SeverityWarning:
			warnCount++
		}
	}

	headers := map[validate.Severity]string{
		validate.SeverityError:   "ERRORS",
		validate.SeverityWarning: "WARNINGS",
	}
	for _, sev := range []validate.Severity{validate.SeverityError, validate.SeverityWarning} {
		header := false
		for _, f := range findings {
			if f.Severity != sev {
				continue
			}
			if !header {
				if _, err := fmt.Fprintf(w, "\n%s:\n", headers[sev]); err != nil {
					return err
				}
				header = true
			}
			loc := ""
			if f.Path != "" {
				loc = " [" + f.Path + "]"
			} else if f.Package != "" {
				loc = " [" + f.Package + "]"
			}
			if _, err := fmt.Fprintf(w, "  - %s: %s%s\n", f.Code, f.Message, loc); err != nil {
				return err
			}
		}
	}

	_, err := fmt.Fprintf(w, "\n%d error(s), %d warning(s).\n", errCount, warnCount)
	return err
}

// writeFindingsJSON emits a stable JSON document. The shape is:
//
//	{
//	  "findings": [<Finding>, ...],
//	  "errors":   <int>,
//	  "warnings": <int>
//	}
//
// Decoupled from the bare []Finding so consumers can parse headline
// counts without iterating.
func writeFindingsJSON(w io.Writer, findings []validate.Finding) error {
	type doc struct {
		Findings []validate.Finding `json:"findings"`
		Errors   int                `json:"errors"`
		Warnings int                `json:"warnings"`
	}
	out := doc{
		Findings: findings,
		Errors:   countSeverity(findings, validate.SeverityError),
		Warnings: countSeverity(findings, validate.SeverityWarning),
	}
	if out.Findings == nil {
		out.Findings = []validate.Finding{}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func countSeverity(findings []validate.Finding, s validate.Severity) int {
	n := 0
	for _, f := range findings {
		if f.Severity == s {
			n++
		}
	}
	return n
}

// ErrExit is a sentinel error wrapping a non-zero exit code. main()
// uses the wrapped value as the process exit code and suppresses the
// "Error: ..." stderr line that a plain error would produce.
//
// Cobra normally renders RunE errors; SilenceErrors on root suppresses
// that, and main inspects the error to set os.Exit. Existing convention
// in this repo: every command returns plain errors. validate is the
// first command that wants a specific non-1 exit code (2 for --strict
// warnings), so we introduce the wrapper here.
type ErrExit int

// Error returns a stable string form. Satisfies the error interface;
// main() doesn't print this. See ExitCode and IsSilentExit.
func (e ErrExit) Error() string { return fmt.Sprintf("exit %d", int(e)) }

// ExitCode reports the exit code an error should map to. Returns 0
// for nil, the wrapped int for any error chain containing ErrExit
// (so callers can wrap-and-still-propagate), and 1 otherwise.
// main() calls this to set os.Exit.
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	var ee ErrExit
	if errors.As(err, &ee) {
		return int(ee)
	}
	return 1
}

// IsSilentExit reports whether the error chain contains an ErrExit.
// main() uses this to skip the default "Error: ..." stderr print for
// errors that are exit-code-only (validate's --strict path emits one).
func IsSilentExit(err error) bool {
	var ee ErrExit
	return errors.As(err, &ee)
}
