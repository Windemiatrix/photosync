-include .env
GOLANGCI_LINT_VERSION ?= v2.15.0
ACTIONLINT_VERSION ?= v1.7.12
GOVULNCHECK_VERSION ?= v1.9.0
SHFMT_VERSION ?= v3.14.1
SVU_VERSION ?= v3.4.1
# Bump together with the GoReleaser version in the release job of ci.yml.
GORELEASER_VERSION ?= v2.18.3
GORELEASER = go run github.com/goreleaser/goreleaser/v2@$(GORELEASER_VERSION)

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-17s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: test
test: mod vet cover lint lint-sh check-repo release-snapshot fixtures-check fixtures-verify ## Run every check (before a PR)

.PHONY: mod
mod: ## go.mod is tidy and the modules match go.sum
	go mod tidy -diff
	go mod verify

.PHONY: vet
vet: ## go vet, the fixtures_verify build tag included
	go vet ./...
	go vet -tags fixtures_verify ./tools/anonymize

# In percent. Not "?=": the environment must not lower it. The per-package
# threshold of 95 % for internal/domain comes with the first package there.
COVERAGE_MIN := 80

.PHONY: cover
cover: ## Tests with the race detector and the coverage threshold
	go test -race -shuffle=on -count=1 -covermode=atomic -coverprofile=coverage.out ./...
	@pct=$$(go tool cover -func=coverage.out | tail -n 1 | tr -dc '0-9.'); \
	echo "coverage: $$pct% (minimum $(COVERAGE_MIN)%)"; \
	test "$${pct%.*}" -ge $(COVERAGE_MIN)

.PHONY: lint
lint: ## Run golangci-lint
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) run ./...

.PHONY: lint-sh
lint-sh: ## shellcheck and the shfmt format check of scripts/ (needs shellcheck installed)
	shellcheck scripts/*.sh
	go run mvdan.cc/sh/v3/cmd/shfmt@$(SHFMT_VERSION) -d -i 2 -ci scripts

.PHONY: fmt
fmt: ## Format the code and the scripts
	gofmt -s -w .
	go run mvdan.cc/sh/v3/cmd/shfmt@$(SHFMT_VERSION) -w -i 2 -ci scripts

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
	go run ./tools/anonymize --check testdata/fixtures

ANONYMIZE_SALT ?= $(HOME)/.config/photosync-dev/anonymize.key

.PHONY: fixtures-verify
fixtures-verify: ## Anonymizer check on the private materials; rebuilt fixtures equal testdata/fixtures (ANONYMIZE_IN, ANONYMIZE_SALT in .env)
	@if [ -z "$(ANONYMIZE_IN)" ]; then \
		echo "fixtures-verify: ANONYMIZE_IN not set, skipped"; \
	else \
		go test -tags fixtures_verify -count=1 -v -run 'TestFixtures' ./tools/anonymize \
			-args -anonymize.in="$(ANONYMIZE_IN)" -anonymize.salt="$(ANONYMIZE_SALT)"; \
	fi

##@ Pull requests

.PHONY: pr-title
pr-title: ## PR_TITLE follows the title convention (make pr-title PR_TITLE='...')
	bash scripts/check-pr-title.sh

##@ Release

# BUMP=major gives 1.0.0: from 0.x the only way there.
BUMP ?= next

.PHONY: next-version
next-version: ## The version that the next merge to main releases (svu, .svu.yml; needs a tag)
	@go run github.com/caarlos0/svu/v3@$(SVU_VERSION) $(BUMP)

.PHONY: release-snapshot
release-snapshot: ## Build the release archive into dist/ without publishing
	$(GORELEASER) release --snapshot --skip=publish --clean
