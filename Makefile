# monorel — task runner.
#
# Run `make` (or `make help`) to list targets.
# Run `make <target>` to invoke one.

GO        ?= go
GOFMT     ?= gofmt
LEFTHOOK  ?= lefthook
BUN       ?= bun

GO_FILES  := $(shell find . -name '*.go' -not -path './docs/node_modules/*' -not -path './node_modules/*')

.DEFAULT_GOAL := help

##@ General

.PHONY: help
help: ## Print this help.
	@awk 'BEGIN { FS = ":.*##"; printf "Usage: make \033[36m<target>\033[0m\n" } \
		/^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2 } \
		/^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) }' \
		$(MAKEFILE_LIST)

##@ Build & Test

.PHONY: build
build: ## Build the monorel binary into ./monorel.
	$(GO) build -o monorel ./cmd/monorel

.PHONY: install
install: ## Install monorel into $$(go env GOPATH)/bin.
	$(GO) install ./cmd/monorel

.PHONY: test
test: ## Run unit tests (no race).
	$(GO) test ./...

.PHONY: test-race
test-race: ## Race tests across the repo. Mirrors pre-push.
	$(GO) test -race -count=1 ./...

##@ Lint & Format

.PHONY: vet
vet: ## go vet ./...
	$(GO) vet ./...

.PHONY: fmt
fmt: ## Format every Go file with gofmt.
	$(GOFMT) -w $(GO_FILES)

.PHONY: fmt-check
fmt-check: ## Fail if any file isn't gofmt-clean.
	@unformatted="$$($(GOFMT) -l $(GO_FILES))"; \
	if [ -n "$$unformatted" ]; then \
		echo "Unformatted files:"; \
		echo "$$unformatted"; \
		echo; \
		echo "Run: make fmt"; \
		exit 1; \
	fi

.PHONY: tidy
tidy: ## go mod tidy.
	$(GO) mod tidy

.PHONY: staticcheck
staticcheck: ## staticcheck ./...
	@if ! command -v staticcheck >/dev/null 2>&1; then \
		echo "staticcheck not on PATH. Install: go install honnef.co/go/tools/cmd/staticcheck@latest" >&2; \
		exit 1; \
	fi
	staticcheck ./...

.PHONY: lint
lint: vet fmt-check staticcheck ## vet + fmt-check + staticcheck.

##@ Docs

.PHONY: docs
docs: ## Build the VitePress docs site (output in docs/.vitepress/dist).
	cd docs && $(BUN) run docs:build

.PHONY: docs-dev
docs-dev: ## Run the VitePress dev server with live reload.
	cd docs && $(BUN) run docs:dev

##@ Tooling

.PHONY: hooks
hooks: ## Install git pre-commit/pre-push hooks via lefthook.
	$(LEFTHOOK) install

.PHONY: toc
toc: ## Regenerate the README table of contents (auto-run by pre-commit).
	$(BUN) run toc

##@ CI

.PHONY: ci
ci: tidy lint test-race ## Full CI gauntlet. Mirror locally before pushing.
