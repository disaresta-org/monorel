# Logging Rules

monorel uses [loglayer-go](https://go.loglayer.dev) for all CLI output. The runtime logger lives at `internal/cli/runtime.go` (`Runtime.Log`); short commands that don't need the full runtime call `newLogger(cmd)` directly. Both paths point the cli transport at `cmd.OutOrStdout()` / `cmd.ErrOrStderr()` so cobra's `SetOut(buffer)` test harness still captures everything.

## Default: write log lines via the logger

Anything that's a single-line status, progress note, hint, "nothing to do" message, or warning routes through the logger. This means it picks up the `--color`, `-v`, and `-q` flag treatment for free, gets sent to the right stream (Info / Debug → stdout, Warn / Error / Fatal → stderr), and stays consistent with the rest of the CLI.

```go
rt.Log.Info("Created release PR #%d: %s", res.PR.Number, res.PR.HTMLURL)
rt.Log.Warn("existing monorel.toml could not be loaded (%v); falling back to auto-detect", err)
rt.Log.Debug("loaded %d changesets from %s", len(rt.Changesets), rt.ChangesetDir)
```

Don't write status / progress messages with `fmt.Fprintln(cmd.OutOrStdout(), ...)`. The bypass case is narrow (next two sections); when in doubt, use the logger.

## Bypass: command's primary output

Lines that constitute the command's RESULT (not chatter ABOUT the result) write directly to `cmd.OutOrStdout()`, bypassing the logger. The reason is `-q`: that flag suppresses Info, and we don't want it silently swallowing the command's actual output.

The greppable headline lines emitted by `apply` / `release` / `tag` / `publish` are the canonical example: the GitHub Action wrapper greps them, and `-q` must not break that contract. Same for `validate` and `doctor`'s text reports — the findings ARE what the user came for; `-q` should suppress the chatter ("No findings...") not the findings themselves.

```go
// ✅ command's primary output: bypass the logger so -q can't suppress it
out := cmd.OutOrStdout()
fmt.Fprintf(out, "Released %d package(s) at %s:\n", len(res.Releases), short(res.CommitSHA))
for _, r := range res.Releases {
    fmt.Fprintf(out, "  %s\n", r.Tag)
}
// ✅ chatter / hint: keep on the logger so -q honors it
rt.Log.Info("Run `git push --follow-tags && monorel publish` to publish.")
```

JSON output paths (`writeFindingsJSON`, `writeDoctorJSON`, etc.) bypass the logger for the same reason plus a stronger one: they're machine-readable structured data, and the cli transport would serialize them as log lines.

## Bypass: multi-line formatted text

The cli transport's sanitizer drops `\n` (defense against log injection) so multi-line `log.X("...\n...\n...")` collapses to one line in the output. Multi-line user-facing text — error messages with "next steps" hints, structured reports, formatted tables outside the logger's table support — writes directly to a writer or returns as a multi-line `error`.

```go
// ✅ multi-line user-facing error: return it; main.go's top-level
// fmt.Fprintln(os.Stderr, err) preserves the line breaks
return fmt.Errorf("not inside a git repository: %s\n\nEither run `monorel init` from inside an existing git checkout, or pass --owner/--repo explicitly:\n\n  monorel init --owner=<your-org> --repo=<your-repo>", dir)

// ❌ would render as one line; the cli transport's sanitizer drops \n
log.Error("not inside a git repository: %s\n\n%s", dir, hint)
```

If the message naturally fits on one line, prefer the logger. The bypass is only for genuinely multi-line shapes.

## Errors: return them, don't log them

The Go convention is "functions return errors; the top of the call stack decides how to display them." monorel follows it: every `RunE` returns an `error`, and `cmd/monorel/main.go` does `fmt.Fprintln(os.Stderr, err)` for non-silent errors.

Don't `log.Error(...)` and then return a sentinel — that double-handles the message and makes error chaining via `errors.Is` / `errors.As` work less cleanly downstream. The exception is when you've ALREADY printed a structured report (validate / doctor) and just need a non-zero exit code — return `ErrExit(1)` to suppress main.go's printer, since it would re-print "exit 1" on top of the already-printed report.

```go
// ✅ normal failure: return the error, let main.go print it
return fmt.Errorf("write monorel.toml: %w", err)

// ✅ already printed the result, just want a non-zero exit
if validate.HasErrors(findings) {
    return ErrExit(1) // main.go's IsSilentExit suppresses the auto-print
}
```

## Levels

| Level | Use for | Stream | Suppressed by |
|-------|---------|--------|---------------|
| `Debug` | Internals visible only with `-v` (loaded N changesets, resolved config path, ...). | stdout | (visible only with `-v`) |
| `Info` | Status, progress, hints, "nothing to do" messages. | stdout | `-q` |
| `Warn` | Non-fatal anomaly the user should notice (config preserved with fallback, deprecation, ...). | stderr | `-q` |
| `Error` | Fatal-but-recoverable condition the user can act on. Rare in `RunE` paths because we prefer to return errors. | stderr | (always shown) |
| `Fatal` | Don't use. Returning an error from `RunE` is the project's pattern; `Fatal` exits the process before deferred cleanups run. | — | — |

## Where loglayer cannot reach

`cmd/monorel/main.go:19`'s `fmt.Fprintln(os.Stderr, err)` is the bootstrap path: errors from cobra flag parsing surface there before any subcommand has constructed a logger. Don't try to plumb a logger through that point — it's the floor of the call stack and the one place raw `fmt` is the right answer.
