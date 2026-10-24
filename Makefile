dummy := $(shell touch .env)
include .env
GOLANGCI_LINT_VERSION ?= v2.15.0
ACTIONLINT_VERSION ?= v1.7.12
GOVULNCHECK_VERSION ?= v1.9.0
GORELEASER_VERSION ?= v2.18.3
GORELEASER_FLAGS ?=
GORELEASER = go run github.com/goreleaser/goreleaser/v2@$(GORELEASER_VERSION)
ANONYMIZE_IN ?=
ANONYMIZE_OUT ?=
# Values from .env are exported only to the targets that need them.

.DEFAULT_GOAL := help

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-17s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: test
test: mod vet cover lint check-repo release-snapshot fixtures-check fixtures-verify ## Run every check (before a PR)

.PHONY: mod
mod: ## go.mod is tidy and the modules match go.sum
	go mod tidy -diff
	go mod verify

.PHONY: vet
vet: ## go vet, the fixtures_verify build tag included
	go vet ./...
	go vet -tags fixtures_verify ./tools/anonymize

.PHONY: cover
cover: ## Tests with the race detector and the coverage thresholds
	go test -race -shuffle=on -count=1 -covermode=atomic -coverprofile=coverage.out ./...
	bash scripts/check-coverage.sh coverage.out

.PHONY: lint
lint: ## Run golangci-lint
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) run ./...

.PHONY: fmt
fmt: ## Format the code
	gofmt -s -w .

##@ Checks

.PHONY: check-repo
check-repo: ## Repository hygiene checks and actionlint
	bash scripts/check-repo.sh
	go run github.com/rhysd/actionlint/cmd/actionlint@$(ACTIONLINT_VERSION)

.PHONY: vuln
vuln: ## Known vulnerabilities in dependencies (not in "make test")
	go run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) ./...

.PHONY: fixtures-check
fixtures-check: ## Leak scan of testdata/fixtures
	@if [ -d testdata/fixtures ]; then go run ./tools/anonymize --check testdata/fixtures; \
	else echo "fixtures-check: no testdata/fixtures, skipped"; fi

.PHONY: fixtures-verify
fixtures-verify: ## Anonymizer check on the private materials (ANONYMIZE_IN, ANONYMIZE_OUT in .env)
	@if [ -z "$(ANONYMIZE_IN)" ] || [ -z "$(ANONYMIZE_OUT)" ]; then \
		echo "fixtures-verify: ANONYMIZE_IN or ANONYMIZE_OUT not set, skipped"; \
	else go test -tags fixtures_verify -count=1 -v -run 'TestFixtures' ./tools/anonymize \
		-args -anonymize.in="$(ANONYMIZE_IN)" -anonymize.out="$(ANONYMIZE_OUT)"; fi

##@ Yandex test account

yandex-token-create yandex-token-delete: export YANDEX_CLIENT_ID := $(YANDEX_CLIENT_ID)
yandex-token-create yandex-token-delete: export YANDEX_CLIENT_SECRET := $(YANDEX_CLIENT_SECRET)

.PHONY: yandex-token-create
yandex-token-create: ## Sign in to the Yandex test account, save the local dev token
	bash scripts/yandex-token.sh create

.PHONY: yandex-token-delete
yandex-token-delete: ## Revoke and delete the local dev token
	bash scripts/yandex-token.sh delete

##@ Pull requests

.PHONY: pr-title
pr-title: ## PR_TITLE follows the title convention (PR_TITLE='...' make pr-title)
	bash scripts/check-pr-title.sh

##@ Release

.PHONY: release-snapshot
release-snapshot: ## Build the release archive into dist/ without publishing
	$(GORELEASER) release --snapshot --skip=publish --clean

.PHONY: release
release: ## Build and create a draft GitHub Release for the current tag (needs GITHUB_TOKEN)
	@header="$$(mktemp "$${RUNNER_TEMP:-$${TMPDIR:-/tmp}}/photosync-release-header.XXXXXX")" && \
	{ cat docs/release-notes-header.md; printf '\n---\n\n'; cat docs/release-notes-header.en.md; } >"$$header" && \
	$(GORELEASER) release --clean --release-header-tmpl "$$header" $(GORELEASER_FLAGS); \
	status=$$?; rm -f "$$header"; exit $$status
