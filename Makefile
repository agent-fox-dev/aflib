.DEFAULT_GOAL := help

DOC_DIR ?= docs/generated

# The golden name and the docs/generated name of the root package.
ROOT_NAME := aflib

# check runs its steps in order and stops at the first failure, also under -j.
.NOTPARALLEL:

.PHONY: help
help: ## Show this help
	@grep -hE '^[a-zA-Z_-]+:.*##' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*##"}; {printf "  %-16s %s\n", $$1, $$2}'

# The build identity make install stamps into the issue command.
VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT     ?= $(shell git rev-parse --short HEAD 2>/dev/null)
BUILD_TIME ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS    := -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildTime=$(BUILD_TIME)

.PHONY: build
build: ## Build all packages and the issue command
	go build ./...

.PHONY: install
install: ## Install the issue command into $GOBIN, stamped with git describe
	go install -ldflags "$(LDFLAGS)" ./cmd/issue

.PHONY: test
test: ## Run all Go tests
	go test ./... -count=1

.PHONY: test-short
test-short: ## Run the Go tests with -short
	go test -short ./...

.PHONY: coverage
coverage: ## Run all tests with a coverage report
	go test ./... -coverprofile=coverage.txt -covermode=atomic
	go tool cover -func=coverage.txt

.PHONY: fmt
fmt: ## gofmt all tracked Go files in place
	@files=$$(git ls-files '*.go'); \
	if [ -n "$$files" ]; then gofmt -l -w $$files; fi

.PHONY: vet
vet: ## go vet ./...
	go vet ./...

.PHONY: lint
lint: ## Run golangci-lint if installed, otherwise skip
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not installed: lint skipped"; \
	fi

.PHONY: check
check: fmt vet lint test-short ## Run fmt, vet, lint and test-short - use before committing

.PHONY: check-release
check-release: fmt vet lint test ## Run fmt, vet, lint and the full test run (manual, before a release or a merge)

.PHONY: doc
doc: ## Generate Go package reference (go doc -all) as text files under docs/generated
	@rm -rf $(DOC_DIR)
	@mkdir -p $(DOC_DIR)
	@mod=$$(go list -m); \
	for p in $$(go list -f '{{if .GoFiles}}{{.ImportPath}}{{end}}' ./... | grep -v -E '/(internal|docs|examples|testdata)(/|$$)'); do \
		rel=$${p#$$mod}; rel=$${rel#/}; \
		name=$$(echo "$${rel:-$(ROOT_NAME)}" | tr / _); \
		go doc -all "$$p" > "$(DOC_DIR)/$$name.txt" || exit 1; \
	done
	@echo "package docs written to $(DOC_DIR)"

.PHONY: tidy
tidy: ## go mod tidy
	go mod tidy

.PHONY: clean
clean: ## Remove build output and the coverage profile
	rm -rf build bin coverage.txt

.PHONY: api-golden
api-golden: ## Regenerate the exported-API goldens in testdata/api
	go test -run TestExportedAPI . -update
