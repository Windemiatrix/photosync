dummy := $(shell touch .env)
include .env
# golangci-lint is built from source by "go run" with the Go toolchain from
# go.mod and verified through the Go checksum database. Bump the version here only.
GOLANGCI_LINT_VERSION ?= v2.15.0
export

.DEFAULT_GOAL := help

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: test
test: ## Vet and run all tests with the race detector
	go vet ./...
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
