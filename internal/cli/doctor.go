package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"

	"github.com/spf13/cobra"

	"monorel.disaresta.com/doctor"
	"monorel.disaresta.com/internal/git"
)

func newDoctorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose repository state issues monorel's planner won't catch.",
		Long: `Runs every built-in repository diagnostic in one pass.

Built-in checks:

  - revived-changeset: a .changeset/*.md file currently on disk that a
    previous chore(release) commit deleted. Likely cause: a contributor
    branched from main BEFORE the release commit and squash-merged
    later; GitHub re-introduced the file. The next release will
    re-ship the same content under a new version unless the file is
    deleted.

Exit codes:
  0  no findings.
  1  one or more error-severity findings.

Pass --json for machine-readable output.`,
		RunE: runDoctor,
	}
	cmd.Flags().Bool("json", false, "Emit findings as JSON.")
	return cmd
}

func runDoctor(cmd *cobra.Command, _ []string) error {
	configPath, err := cmd.Flags().GetString(configPathFlagName)
	if err != nil {
		return err
	}
	asJSON, _ := cmd.Flags().GetBool("json")

	abs, err := filepath.Abs(configPath)
	if err != nil {
		return fmt.Errorf("resolve --config: %w", err)
	}
	repoDir := filepath.Dir(abs)
	repo := git.Open(repoDir)

	findings, err := doctor.Run(doctor.Options{
		RepoDir: repoDir,
		GitLog:  repo.DeletedFilesInCommitsMatching,
	})
	if err != nil {
		return err
	}

	if asJSON {
		if err := writeDoctorJSON(cmd.OutOrStdout(), findings); err != nil {
			return err
		}
	} else {
		if err := writeDoctorText(cmd.OutOrStdout(), findings); err != nil {
			return err
		}
	}

	if hasDoctorErrors(findings) {
		return ErrExit(1)
	}
	return nil
}

func writeDoctorText(w io.Writer, findings []doctor.Finding) error {
	if len(findings) == 0 {
		_, err := fmt.Fprintln(w, "No findings. Repository state looks healthy.")
		return err
	}
	errCount := 0
	warnCount := 0
	for _, f := range findings {
		switch f.Severity {
		case doctor.SeverityError:
			errCount++
		case doctor.SeverityWarning:
			warnCount++
		}
	}
	for _, sev := range []doctor.Severity{doctor.SeverityError, doctor.SeverityWarning} {
		header := false
		for _, f := range findings {
			if f.Severity != sev {
				continue
			}
			if !header {
				label := "ERRORS"
				if sev == doctor.SeverityWarning {
					label = "WARNINGS"
				}
				if _, err := fmt.Fprintf(w, "\n%s:\n", label); err != nil {
					return err
				}
				header = true
			}
			loc := ""
			if f.Path != "" {
				loc = " [" + f.Path + "]"
			}
			if _, err := fmt.Fprintf(w, "  - %s: %s%s\n", f.CheckName, f.Message, loc); err != nil {
				return err
			}
		}
	}
	_, err := fmt.Fprintf(w, "\n%d error(s), %d warning(s).\n", errCount, warnCount)
	return err
}

// writeDoctorJSON emits the findings + headline counts. The shape
// matches validate's `{findings, errors, warnings}` envelope; the
// per-finding object uses `check_name` (not `code`) because doctor
// findings name the diagnostic that produced them, not a stable
// lint identifier. Severity values are `"error"` and `"warning"`,
// matching validate's wire form.
func writeDoctorJSON(w io.Writer, findings []doctor.Finding) error {
	type wireFinding struct {
		Severity  doctor.Severity `json:"severity"`
		CheckName string          `json:"check_name"`
		Path      string          `json:"path,omitempty"`
		Message   string          `json:"message"`
	}
	type doc struct {
		Findings []wireFinding `json:"findings"`
		Errors   int           `json:"errors"`
		Warnings int           `json:"warnings"`
	}
	out := doc{Findings: make([]wireFinding, 0, len(findings))}
	for _, f := range findings {
		out.Findings = append(out.Findings, wireFinding{
			Severity:  f.Severity,
			CheckName: f.CheckName,
			Path:      f.Path,
			Message:   f.Message,
		})
		switch f.Severity {
		case doctor.SeverityError:
			out.Errors++
		case doctor.SeverityWarning:
			out.Warnings++
		}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// hasDoctorErrors reports whether any finding is error-severity.
// All current built-in checks emit SeverityError; when a warning-
// only check ships, add a `--strict` flag mirroring validate to let
// callers fail closed on warnings too.
func hasDoctorErrors(findings []doctor.Finding) bool {
	for _, f := range findings {
		if f.Severity == doctor.SeverityError {
			return true
		}
	}
	return false
}
