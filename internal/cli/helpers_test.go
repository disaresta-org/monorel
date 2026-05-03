package cli

import (
	"bytes"
	"io"

	"go.loglayer.dev/plugins/fmtlog/v2"
	"go.loglayer.dev/transports/cli/v2"
	loglayer "go.loglayer.dev/v2"
)

// testLogger returns a logger whose info/debug AND warn/error/fatal
// output both flow into a single buffer. Useful when a test wants
// "everything monorel printed" without sorting stdout from stderr.
// The cli transport's color resolution is forced off so assertions
// can match plain text without scrubbing ANSI escapes.
func testLogger() (*loglayer.LogLayer, *bytes.Buffer) {
	var buf bytes.Buffer
	return loggerToWriter(&buf, &buf), &buf
}

// loggerToWriter constructs a logger that routes info/debug to
// stdout and warn/error/fatal to stderr (the io.Writers passed in).
// Used when a test needs to keep the two streams separate.
func loggerToWriter(stdout, stderr io.Writer) *loglayer.LogLayer {
	t := cli.New(cli.Config{
		Stdout: stdout,
		Stderr: stderr,
		Color:  cli.ColorNever,
	})
	log := loglayer.New(loglayer.Config{
		Transport: t,
		Plugins:   []loglayer.Plugin{fmtlog.New()},
	})
	log.SetLevel(loglayer.LogLevelInfo)
	return log
}
