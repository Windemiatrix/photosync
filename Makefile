dummy := $(shell touch .env)
include .env
# golangci-lint is built from source by "go run" with the Go toolchain from
# go.mod and verified through the Go checksum database. Bump the version here only.
GOLANGCI_LINT_VERSION ?= v2.15.0
# GoReleaser is run the same way. Bump the version here only.
GORELEASER_VERSION ?= v2.18.3
# Extra flags for "make release", for example GORELEASER_FLAGS=--skip=publish to
# build the release notes on a temporary local tag without a token.
GORELEASER_FLAGS ?=
GORELEASER = go run github.com/goreleaser/goreleaser/v2@$(GORELEASER_VERSION)
# Private anonymizer inputs, set in .env (never committed): the manual-sync
# materials and the fixtures built from them. Without them the checks on real
# data are skipped.
ANONYMIZE_IN ?=
ANONYMIZE_OUT ?=
export

.DEFAULT_GOAL := help

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-17s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: test
test: unit lint check-repo release-check fixtures-check fixtures-verify ## Run every local check (before a PR)

.PHONY: unit
unit: ## Vet and run all tests with the race detector
	go vet ./...
	go vet -tags fixtures_verify ./tools/anonymize
	go test -race -shuffle=on -count=1 ./...

.PHONY: cover
cover: ## Run tests with coverage and check the thresholds
	go test -race -shuffle=on -covermode=atomic -coverprofile=coverage.out ./...
	bash scripts/check-coverage.sh coverage.out

.PHONY: lint
lint: ## Run golangci-lint
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) run ./...

.PHONY: fmt
fmt: ## Format the code
	gofmt -s -w .

##@ Checks

.PHONY: check-repo
check-repo: ## Repository hygiene checks (the repo-checks CI job without actionlint)
	bash scripts/check-repo.sh

.PHONY: fixtures-check
fixtures-check: ## Scan testdata/fixtures for leaks (skipped while there are none)
	@if [ -d testdata/fixtures ]; then go run ./tools/anonymize --check testdata/fixtures; \
	else echo "fixtures-check: no testdata/fixtures, skipped"; fi

# The anonymizer output on the private materials: the same pairs and levels
# before and after, no input word of 4+ letters in the fixtures, and a digest
# of at most 30 anonymized records to read. Prints aggregates only.
.PHONY: fixtures-verify
fixtures-verify: ## Check the anonymizer on the private materials (needs ANONYMIZE_IN and ANONYMIZE_OUT in .env)
	@if [ -z "$(ANONYMIZE_IN)" ] || [ -z "$(ANONYMIZE_OUT)" ]; then \
		echo "fixtures-verify: ANONYMIZE_IN or ANONYMIZE_OUT not set, skipped"; \
	else go test -tags fixtures_verify -count=1 -v -run 'TestFixtures' ./tools/anonymize \
		-args -anonymize.in="$(ANONYMIZE_IN)" -anonymize.out="$(ANONYMIZE_OUT)"; fi

##@ Release

# GoReleaser reads .goreleaser.yaml. The tag itself is made by scripts/release.sh.

.PHONY: release-check
release-check: ## Validate .goreleaser.yaml
	$(GORELEASER) check

.PHONY: release-snapshot
release-snapshot: ## Build the release archive into dist/ without publishing
	$(GORELEASER) release --snapshot --skip=publish --clean

# The notes header is the Russian and the English file joined into one
# temporary file. It must live outside the work tree (an untracked file would
# make the git state dirty, and GoReleaser would refuse after the tag is
# pushed) and outside dist/ (--clean empties it before the header is read), so
# it goes under $RUNNER_TEMP in CI or the temporary directory locally, and is
# removed afterwards whatever the result.
.PHONY: release
release: ## Build and create a draft GitHub Release for the current tag (needs GITHUB_TOKEN)
	@header="$$(mktemp "$${RUNNER_TEMP:-$${TMPDIR:-/tmp}}/photosync-release-header.XXXXXX")" && \
	{ cat docs/release-notes-header.md; printf '\n---\n\n'; cat docs/release-notes-header.en.md; } >"$$header" && \
	$(GORELEASER) release --clean --release-header-tmpl "$$header" $(GORELEASER_FLAGS); \
	status=$$?; rm -f "$$header"; exit $$status
