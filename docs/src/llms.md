---
title: Use with AI / LLMs
description: monorel ships LLM-friendly reference files following the llmstxt.org convention.
---

# Use with AI / LLMs

monorel ships two reference files designed to be pasted into a chat with Claude, ChatGPT, Cursor, Copilot Chat, or any other LLM-backed coding assistant. They follow the [llmstxt.org](https://llmstxt.org) convention.

| File | Purpose | Size |
|------|---------|------|
| **[/llms.txt](https://monorel.disaresta.com/llms.txt)** | Concise index: the lifecycle, every command, the file formats, and the public library API in a single file. Use this when context is precious. | small |
| **[/llms-full.txt](https://monorel.disaresta.com/llms-full.txt)** | Comprehensive single-file reference: full command flags, library API signatures, CI pipeline templates per provider, recovery patterns, operational guarantees and limitations. Use this when context is cheap. | medium |

Both files are kept in sync with the docs site on every release.

## How to use them

### With Claude / ChatGPT (web)

Paste the contents of `llms-full.txt` (or its URL) at the start of your conversation:

> Use the monorel reference at https://monorel.disaresta.com/llms-full.txt as authoritative.
> Then write me a `.gitlab-ci.yml` that uses the published Docker image and adds a pre-merge `monorel doctor` job.

### With Cursor / Continue / similar IDE assistants

Add the URL as a context source. Most IDE assistants accept a URL or local file as a "doc source": drop in `https://monorel.disaresta.com/llms-full.txt` and the model will reference it when answering monorel-related questions.

### With Claude Code / Aider / SDK-based tools

Fetch the file once and include it as a system message:

```sh
curl -sSL https://monorel.disaresta.com/llms-full.txt > .ai/monorel.txt
```

Then add `.ai/monorel.txt` to your tool's context-files list (or wire it into a custom system prompt).

## Sample prompts

Once the reference is loaded, these prompts produce useful output:

- *"Write a `.changeset/<name>.md` for a change that adds a new public function `Foo()` to `transports/zerolog`. Bump-level your call."*
- *"Generate a complete `monorel.toml` for a repo with a root module at `go.example.com/widget` plus three sub-modules under `transports/`."*
- *"Write a GitHub Actions workflow that runs `monorel doctor` on every PR and adds a status check to the always-open release PR."*
- *"Explain why merging a release PR via squash-merge instead of fast-forward breaks the `chore(release):` trailer convention."*
- *"Write a small Go program that uses `monorel.disaresta.com/plan` to render the next release plan as JSON, given a `.changeset/` directory and the current tag list."*
- *"My release PR's diff shows a `go.sum` change I didn't expect. What's in `monorel apply` that would touch it?"*

## Why two files?

`llms.txt` is for cases where context is precious: it's a sitemap that lets the model fetch only what it needs. `llms-full.txt` is for cases where context is cheap (long-context models, agentic tools): everything is inlined so the model doesn't need to browse.

If you're not sure, start with `llms-full.txt`. It's the lower-friction option.

## Other ways to feed monorel to an LLM

- **Source code** at [github.com/disaresta-org/monorel](https://github.com/disaresta-org/monorel) is small enough to fit in most coding assistants' context.
- **pkg.go.dev** (`pkg.go.dev/monorel.disaresta.com`) renders all GoDoc for the public library packages, including type signatures and doc comments. Useful for fact-checking the model's output.
- **This docs site** is itself indexed by most search-augmented assistants. Asking "from the monorel docs, ..." often works without any setup.
